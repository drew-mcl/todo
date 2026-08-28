import AppKit

// The capture window: your shorthand on the top half, what it is about to
// become on the bottom. Every keystroke is re-read, which is the whole reason
// this exists rather than a text field and a save button.

/// A button drawn in this window's colours.
///
/// Not an NSButton. AppKit paints one in the system accent -- whatever blue the
/// machine is set to -- and on recent macOS it is a hosted view that draws the
/// whole control, so neither the bezel colour nor an override of draw(_:) can
/// talk it out of it. The palette everything else here comes from wins instead.
final class Pill: NSView {
    var fill: NSColor = .controlAccentColor { didSet { needsDisplay = true } }
    var label = NSAttributedString() { didSet { needsDisplay = true } }
    var isEnabled = true { didSet { needsDisplay = true } }
    var onClick: (() -> Void)?

    override var intrinsicContentSize: NSSize {
        NSSize(width: max(56, label.size().width + 26), height: 26)
    }

    override func draw(_ dirtyRect: NSRect) {
        fill.setFill()
        NSBezierPath(roundedRect: bounds, xRadius: 6, yRadius: 6).fill()
        let size = label.size()
        label.draw(at: NSPoint(x: (bounds.width - size.width) / 2,
                               y: (bounds.height - size.height) / 2))
    }

    override func mouseDown(with event: NSEvent) {
        guard isEnabled else { return }
        onClick?()
    }

    override func isAccessibilityElement() -> Bool { true }
    override func accessibilityRole() -> NSAccessibility.Role? { .button }
    override func accessibilityLabel() -> String? { label.string }
}

final class CaptureController: NSObject, NSTextViewDelegate, NSWindowDelegate {
    let panel = CapturePanel()

    private let bridge: Bridge
    private let titleField = NSTextField()
    private let summary = NSTextField(labelWithString: "")
    private let hint = NSTextField(labelWithString: "")
    private let modeLabel = NSTextField(labelWithString: "")
    private let addButton = Pill()
    private var draft: NSTextView!
    private var previewView: NSTextView!
    private var previewScroll: NSScrollView!

    private var latest: Preview?
    private var hues: [String: Int] = [:]
    private var awaiting = 0
    private var debounce: Timer?
    private var settle: Timer?
    private var lastTyped = Date.distantPast
    private var monitor: Any?
    private var notice: NSAttributedString?
    private var vim = VimState(text: "", at: 0)
    private var sheet: NSTextView?
    private var takeBack: TakeBack?
    private var previousApp: NSRunningApplication?

    /// The last thing this window did, and the draft it did it to.
    ///
    /// Closing files what you wrote, so there has to be one key that means "no,
    /// put that back" -- and it has to put the words back too, not just take
    /// the tasks off the list, or you are left retyping the thing you were
    /// trying to correct.
    private struct TakeBack {
        let draft: String
        let title: String
        let batch: Int?   // nil when the draft was scrapped rather than filed
        let count: Int
        let at: Date
    }

    /// How long the offer to take it back stands. Long enough to reopen the
    /// window and change your mind, short enough not to offer to undo something
    /// from another conversation.
    private static let secondThoughts: TimeInterval = 300

    init(bridge: Bridge) {
        self.bridge = bridge
        super.init()
        build()
        panel.delegate = self
    }

    // ── the window ──────────────────────────────────────────────────────────

    var isShowing: Bool { panel.isVisible }

    func toggle() { isShowing && panel.isKeyWindow ? hide() : show() }

    func show() {
        previousApp = NSWorkspace.shared.frontmostApplication
        if !panel.isVisible { panel.positionOnActiveScreen() }
        panel.level = .floating
        NSApp.activate(ignoringOtherApps: true)
        panel.makeKeyAndOrderFront(nil)
        panel.makeFirstResponder(draft)
        draft.setSelectedRange(NSRange(location: draft.string.utf16.count, length: 0))
        lastTyped = .distantPast
        vim = VimState(text: draft.string, at: draft.selectedRange().location)
        showMode()
        installMonitor()
        reparse()
        offerTakeBack()
    }

    func hide() {
        removeMonitor()
        panel.orderOut(nil)
        // Back to whatever the note was about, without a trip through the Dock.
        previousApp?.activate()
        previousApp = nil
    }

    /// Clicking away does not close this window and does not empty it.
    ///
    /// That is deliberate: the reason to reach for a capture box mid-call is to
    /// paste something into it, and going to fetch that something must not cost
    /// you what you have already typed. It only stops floating over everything,
    /// so it is not in the way while you are gone.
    func windowDidResignKey(_ notification: Notification) {
        panel.level = .normal
        removeMonitor()
    }

    func windowDidBecomeKey(_ notification: Notification) {
        panel.level = .floating
        installMonitor()
    }

    // ── keys ────────────────────────────────────────────────────────────────

    private func installMonitor() {
        guard monitor == nil else { return }
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            self?.handle(event) == true ? nil : event
        }
    }

    private func removeMonitor() {
        if let monitor { NSEvent.removeMonitor(monitor) }
        monitor = nil
    }

    /// Returns true when the key has been dealt with and should go no further.
    /// What each one means is in Keys, so the mapping can be read -- and
    /// checked -- without a window.
    private func handle(_ event: NSEvent) -> Bool {
        guard panel.isKeyWindow else { return false }
        if showingSheet {
            // Anything at all puts the reference away.
            hideSheet()
            return true
        }
        let command = event.modifierFlags.contains(.command)
        if command && event.charactersIgnoringModifiers == "/" {
            showSheet()
            return true
        }
        // Normal mode has the keyboard before the window's own bindings do,
        // and hands back the two that mean leaving.
        if !command, titleField.currentEditor() == nil, let typed = event.characters,
           vim.mode == .normal || typed == "\u{1b}" {
            if runVim(typed) { return true }
        }
        switch Keys.press(code: event.keyCode,
                          command: command,
                          inTitle: titleField.currentEditor() != nil,
                          canTakeBack: canTakeBack) {
        case .file:
            fileAndClose()
        case .scrap:
            scrap()
        case .reverse:
            reverse()
        case .leaveTitle:
            panel.makeFirstResponder(draft)
        case .today:
            hide()
            NotificationCenter.default.post(name: .todoToday, object: nil)
        case .pass:
            return false
        }
        return true
    }

    /// Hand a keystroke to normal mode and put back what comes out. Returns
    /// false when the key was not its business.
    private func runVim(_ typed: String) -> Bool {
        vim.text = draft.string
        vim.at = draft.selectedRange().location

        let out = Vim.press(typed, vim)
        guard out.handled else { return false }
        vim = out.state

        if out.state.text != draft.string {
            // Through the text view, so its own undo still holds the line you
            // just deleted.
            let whole = NSRange(location: 0, length: (draft.string as NSString).length)
            if draft.shouldChangeText(in: whole, replacementString: out.state.text) {
                draft.textStorage?.replaceCharacters(in: whole, with: out.state.text)
                draft.didChangeText()
            }
        }
        draft.setSelectedRange(NSRange(location: min(out.state.at, (draft.string as NSString).length),
                                       length: 0))
        showMode()

        switch out.exit {
        case .file: fileAndClose()
        case .scrap: scrap()
        case .help: showSheet()
        case nil: break
        }
        return true
    }

    /// A modal editor that will not say which mode it is in is the oldest bad
    /// joke in software, so it says so twice: in the top bar, and in the colour
    /// of the caret you are looking at anyway.
    private func showMode() {
        let normal = vim.mode == .normal
        draft.insertionPointColor = Theme.shared.colour(normal ? "accent" : "ink")
        modeLabel.attributedStringValue = NSAttributedString(
            string: normal ? "NORMAL" : "INSERT",
            attributes: [
                .font: NSFont.monospacedSystemFont(ofSize: 10, weight: .semibold),
                .foregroundColor: Theme.shared.colour(normal ? "accent" : "ink4"),
                .kern: 1.2,
            ])
        modeLabel.toolTip = normal
            ? "esc files what is there · ⌘/ for the keys"
            : "esc stops typing · ⌘/ for the keys"
        drawChrome()
    }

    private var canTakeBack: Bool {
        guard let takeBack else { return false }
        return Date().timeIntervalSince(takeBack.at) < Self.secondThoughts
    }

    // ── reading the draft ───────────────────────────────────────────────────

    func textDidChange(_ notification: Notification) {
        // Anything the window was saying is about the draft as it was.
        notice = nil
        // A line being typed is not told off until you have stopped. The
        // complaint is still coming -- it just waits for you to finish.
        lastTyped = Date()
        settle?.invalidate()
        settle = Timer.scheduledTimer(withTimeInterval: Self.quietFor + 0.02, repeats: false) {
            [weak self] _ in self?.render()
        }
        // Debounced, because the answer comes from another process; short
        // enough that the parse still lands while you are looking at the line.
        debounce?.invalidate()
        debounce = Timer.scheduledTimer(withTimeInterval: 0.07, repeats: false) { [weak self] _ in
            self?.reparse()
        }
    }

    func textViewDidChangeSelection(_ notification: Notification) {
        // Only the marker moved, and the parse has not changed.
        render()
    }

    private func reparse() {
        let text = draft.string
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            latest = nil
            hues = [:]
            render()
            return
        }
        awaiting = bridge.send("preview", draft: text) { [weak self] reply in
            guard let self, reply.id == self.awaiting else { return }
            if let preview = reply.preview {
                self.latest = preview
                self.hues = reply.hues ?? [:]
            }
            self.render()
        }
    }

    /// File what the draft amounts to and get out of the way.
    ///
    /// A draft with nothing fileable in it is not an error and is not thrown
    /// away: the window simply closes and keeps it, because half a thought you
    /// have not finished typing is the thing here worth losing least.
    private func fileAndClose() {
        guard let latest, latest.tasks > 0 else {
            hide()
            return
        }
        let words = draft.string, called = titleField.stringValue
        bridge.send("capture", draft: words, title: called) { [weak self] reply in
            guard let self else { return }
            if let error = reply.error {
                // Something refused it, so the window stays and says why rather
                // than closing over the top of the reason.
                self.flash(error)
                return
            }
            guard let added = reply.added else { return }
            self.takeBack = TakeBack(draft: words, title: called,
                                     batch: added.batchId, count: added.added, at: Date())
            self.empty()
            self.hide()
        }
    }

    /// Throw the draft away. Recoverable for as long as second thoughts last,
    /// because a window that empties itself on one keystroke had better be.
    private func scrap() {
        if !draft.string.isEmpty || !titleField.stringValue.isEmpty {
            takeBack = TakeBack(draft: draft.string, title: titleField.stringValue,
                                batch: nil, count: 0, at: Date())
        }
        empty()
        hide()
    }

    /// Put back whatever the last keystroke did: the tasks come off the list
    /// and the words go back in the box, which is the state you were in.
    private func reverse() {
        guard let previous = takeBack else { return }
        takeBack = nil

        draft.string = previous.draft
        titleField.stringValue = previous.title
        draft.setSelectedRange(NSRange(location: previous.draft.utf16.count, length: 0))
        reparse()

        guard let batch = previous.batch else {
            say("put back")
            return
        }
        bridge.send("undo", batch: batch) { [weak self] reply in
            guard let self else { return }
            if let error = reply.error {
                self.flash(error)
                return
            }
            self.say("took back \(reply.undone ?? previous.count)")
        }
    }

    private func empty() {
        draft.string = ""
        titleField.stringValue = ""
        latest = nil
        hues = [:]
        notice = nil
        render()
    }

    /// What the window last did, offered back until you type over it.
    private func offerTakeBack() {
        guard canTakeBack, let takeBack else { return }
        if takeBack.batch == nil {
            say("scrapped · ⌘Z to put it back")
        } else {
            say("filed \(takeBack.count) · ⌘Z to take it back")
        }
    }

    private func say(_ message: String) {
        notice = NSAttributedString(string: message, attributes: [
            .font: Type.mono, .foregroundColor: Theme.shared.colour("ink3"),
        ])
        drawChrome()
    }

    private func flash(_ message: String) {
        notice = NSAttributedString(string: message, attributes: [
            .font: Type.mono, .foregroundColor: Theme.shared.colour("danger"),
        ])
        drawChrome()
    }

    // ── drawing ─────────────────────────────────────────────────────────────

    /// Redraw with whatever is known now -- called when the colours arrive.
    func repaint() { render() }

    /// Feed a key straight to normal mode. The window cannot hold the keyboard
    /// when it is only being drawn to a file, so `make shot` says so directly.
    func pressForShot(_ key: String) { _ = runVim(key) }

    private func render() {
        colourTheDraft()
        drawPreview()
        drawChrome()
    }

    /// The line the caret is on, counting from one the way the parser does.
    private var caretLine: Int {
        let upTo = (draft.string as NSString)
            .substring(to: min(draft.selectedRange().location, draft.string.utf16.count))
        return upTo.components(separatedBy: "\n").count
    }

    /// Paint the grammar in place, using the spans the parser handed back. The
    /// text is never rewritten -- only its colour -- so the caret does not move.
    private func colourTheDraft() {
        guard let storage = draft.textStorage else { return }
        storage.beginEditing()
        storage.setAttributes(
            [.font: Type.draft, .foregroundColor: Theme.shared.colour("ink2")],
            range: NSRange(location: 0, length: storage.length))
        for (range, kind) in Render.spans(for: draft.string, lines: latest?.lines ?? []) {
            storage.addAttribute(.foregroundColor, value: Theme.shared.token(kind), range: range)
        }
        storage.endEditing()

        // Anything typed next starts plain and is coloured when it is read.
        draft.typingAttributes = [
            .font: Type.draft, .foregroundColor: Theme.shared.colour("ink2"),
        ]
    }

    /// How long the line under the caret gets before it is told what is wrong
    /// with it. Long enough not to interrupt a word, short enough to be help.
    private static let quietFor: TimeInterval = 0.7

    private func drawPreview() {
        guard let storage = previewView.textStorage else { return }
        let settled = Date().timeIntervalSince(lastTyped) > Self.quietFor
        let (text, marked) = Render.preview(latest, hues: hues, caret: caretLine, settled: settled)
        storage.setAttributedString(text)
        // Follow the line being typed rather than sitting at the top of a page
        // you typed several minutes ago.
        if let marked { previewView.scrollRangeToVisible(marked) }
    }

    private func drawChrome() {
        summary.attributedStringValue = Render.heading(Render.summary(latest))

        let ready = (latest?.tasks ?? 0) > 0
        addButton.isEnabled = ready
        addButton.label = NSAttributedString(
            string: latest.map { $0.tasks > 0 ? "file \($0.tasks)" : "file" } ?? "file",
            attributes: [
                .font: NSFont.monospacedSystemFont(ofSize: 12, weight: .medium),
                .foregroundColor: Theme.shared.colour(ready ? "sunk" : "ink4"),
            ])
        addButton.fill = Theme.shared.colour(ready ? "accent" : "line")
        addButton.invalidateIntrinsicContentSize()

        // A task line typed into the title box is the one mistake this shape of
        // window invites, so it is named rather than left to look like a parser
        // that has stopped reading pipes.
        if titleField.stringValue.contains("|") {
            hint.attributedStringValue = NSAttributedString(
                string: "that is a task line, not a title — it belongs below",
                attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("danger")])
        } else {
            hint.attributedStringValue = notice ?? Render.grammar()
        }
    }

    // ── the reference ───────────────────────────────────────────────────────

    private var showingSheet: Bool { sheet?.isHidden == false }

    /// The keys, over the top of the box, from the list the bridge handed over.
    /// Any key at all puts it away again.
    private func showSheet() {
        let view = sheet ?? {
            let (scroll, text) = Self.textView(editable: false)
            text.textContainerInset = NSSize(width: 24, height: 20)
            scroll.translatesAutoresizingMaskIntoConstraints = false
            scroll.drawsBackground = true
            scroll.backgroundColor = Theme.shared.colour("sunk")
            panel.contentView?.addSubview(scroll)
            NSLayoutConstraint.activate([
                scroll.topAnchor.constraint(equalTo: panel.contentView!.topAnchor),
                scroll.bottomAnchor.constraint(equalTo: panel.contentView!.bottomAnchor),
                scroll.leadingAnchor.constraint(equalTo: panel.contentView!.leadingAnchor),
                scroll.trailingAnchor.constraint(equalTo: panel.contentView!.trailingAnchor),
            ])
            sheet = text
            return text
        }()
        view.textStorage?.setAttributedString(Render.sheet(Theme.shared.keys))
        view.enclosingScrollView?.isHidden = false
        view.isHidden = false
    }

    private func hideSheet() {
        sheet?.isHidden = true
        sheet?.enclosingScrollView?.isHidden = true
        panel.makeFirstResponder(draft)
    }

    // ── layout ──────────────────────────────────────────────────────────────

    private func build() {
        guard let root = panel.contentView else { return }

        titleField.placeholderString = "header (optional)"
        titleField.toolTip = "Names this capture, so you can find it again under calls."
        titleField.font = Type.mono
        titleField.isBordered = false
        titleField.drawsBackground = false
        titleField.focusRingType = .none
        titleField.target = self
        titleField.action = #selector(titleCommitted)
        titleField.delegate = self

        let (draftScroll, draftView) = Self.textView(editable: true)
        draft = draftView
        draft.delegate = self
        draft.font = Type.draft
        draft.textContainerInset = NSSize(width: 18, height: 14)

        let (previewScrollView, preview) = Self.textView(editable: false)
        previewView = preview
        previewScroll = previewScrollView
        previewView.textContainerInset = NSSize(width: 18, height: 10)

        let divider = NSBox()
        divider.boxType = .separator
        let footRule = NSBox()
        footRule.boxType = .separator
        let headRule = NSBox()
        headRule.boxType = .separator

        summary.attributedStringValue = Render.heading("NOTHING YET")
        hint.attributedStringValue = Render.grammar()
        hint.lineBreakMode = .byTruncatingTail

        // Not the default button. macOS paints that one in the system accent,
        // which is whatever blue the machine is set to and nothing to do with
        // the palette everything else here is drawn from. ⌘↵ still files -- the
        // key monitor has it, and had it before this button ever saw the event.
        // Drawn, not bezelled. A push button is painted in the system accent --
        // whatever blue the machine is set to -- and a default button doubly so,
        // neither of which has anything to do with the palette the rest of this
        // window is drawn from. ⌘↵ still files: the key monitor has it.
        addButton.onClick = { [weak self] in self?.fileAndClose() }

        let keys = NSTextField(
            labelWithString: "esc file · ⌘⌫ scrap · ⌘T today · ⌘/ keys")
        keys.font = Type.mono
        keys.textColor = Theme.shared.colour("ink4")

        modeLabel.toolTip = "esc stops typing · ⌘/ for the keys"

        let views: [NSView] = [
            titleField, modeLabel, headRule, draftScroll, divider, summary,
            previewScrollView, footRule, hint, keys, addButton,
        ]
        for v in views {
            v.translatesAutoresizingMaskIntoConstraints = false
            root.addSubview(v)
        }

        let pad: CGFloat = 20
        NSLayoutConstraint.activate([
            titleField.topAnchor.constraint(equalTo: root.topAnchor, constant: 16),
            titleField.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad),
            titleField.trailingAnchor.constraint(
                equalTo: modeLabel.leadingAnchor, constant: -12),

            modeLabel.centerYAnchor.constraint(equalTo: titleField.centerYAnchor),
            modeLabel.trailingAnchor.constraint(equalTo: root.trailingAnchor, constant: -pad),

            headRule.topAnchor.constraint(equalTo: titleField.bottomAnchor, constant: 12),
            headRule.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            headRule.trailingAnchor.constraint(equalTo: root.trailingAnchor),

            draftScroll.topAnchor.constraint(equalTo: headRule.bottomAnchor),
            draftScroll.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            draftScroll.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            draftScroll.heightAnchor.constraint(equalTo: root.heightAnchor, multiplier: 0.34),

            divider.topAnchor.constraint(equalTo: draftScroll.bottomAnchor),
            divider.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            divider.trailingAnchor.constraint(equalTo: root.trailingAnchor),

            summary.topAnchor.constraint(equalTo: divider.bottomAnchor, constant: 10),
            summary.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad + 2),
            summary.trailingAnchor.constraint(equalTo: root.trailingAnchor, constant: -pad),

            previewScrollView.topAnchor.constraint(equalTo: summary.bottomAnchor, constant: 4),
            previewScrollView.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            previewScrollView.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            previewScrollView.bottomAnchor.constraint(equalTo: footRule.topAnchor),

            footRule.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            footRule.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            footRule.bottomAnchor.constraint(equalTo: addButton.topAnchor, constant: -10),

            hint.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad + 2),
            hint.centerYAnchor.constraint(equalTo: addButton.centerYAnchor),
            hint.trailingAnchor.constraint(lessThanOrEqualTo: keys.leadingAnchor, constant: -12),

            keys.trailingAnchor.constraint(equalTo: addButton.leadingAnchor, constant: -12),
            keys.centerYAnchor.constraint(equalTo: addButton.centerYAnchor),

            addButton.trailingAnchor.constraint(equalTo: root.trailingAnchor, constant: -pad),
            addButton.bottomAnchor.constraint(equalTo: root.bottomAnchor, constant: -14),
        ])
    }

    @objc private func titleCommitted() { panel.makeFirstResponder(draft) }

    /// The one incantation for a text view inside a scroll view, with every
    /// helpful substitution turned off: smart quotes and smart dashes rewrite
    /// the very characters the grammar is made of.
    static func textView(editable: Bool) -> (NSScrollView, NSTextView) {
        let scroll = NSScrollView()
        scroll.hasVerticalScroller = true
        scroll.drawsBackground = false
        scroll.autohidesScrollers = true
        scroll.borderType = .noBorder

        let view = NSTextView()
        view.minSize = NSSize(width: 0, height: 0)
        let unbounded = CGFloat.greatestFiniteMagnitude
        view.maxSize = NSSize(width: unbounded, height: unbounded)
        view.isVerticallyResizable = true
        view.isHorizontallyResizable = false
        view.autoresizingMask = [.width]
        view.textContainer?.widthTracksTextView = true
        view.textContainer?.containerSize = NSSize(width: 0, height: unbounded)
        view.drawsBackground = false
        view.isRichText = false
        view.usesFontPanel = false
        view.allowsUndo = editable
        view.isEditable = editable
        view.isSelectable = true
        view.font = Type.mono
        view.isAutomaticQuoteSubstitutionEnabled = false
        view.isAutomaticDashSubstitutionEnabled = false
        view.isAutomaticTextReplacementEnabled = false
        view.isAutomaticSpellingCorrectionEnabled = false
        view.isContinuousSpellCheckingEnabled = false
        view.isGrammarCheckingEnabled = false
        view.isAutomaticLinkDetectionEnabled = false
        view.isAutomaticDataDetectionEnabled = false

        scroll.documentView = view
        return (scroll, view)
    }
}

extension CaptureController: NSTextFieldDelegate {
    func controlTextDidChange(_ notification: Notification) { drawChrome() }
}
