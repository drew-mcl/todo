import AppKit

// The capture window: your shorthand on the top half, what it is about to
// become on the bottom. Every keystroke is re-read, which is the whole reason
// this exists rather than a text field and a save button.

final class CaptureController: NSObject, NSTextViewDelegate, NSWindowDelegate {
    let panel = CapturePanel()

    private let bridge: Bridge
    private let titleField = NSTextField()
    private let summary = NSTextField(labelWithString: "")
    private let hint = NSTextField(labelWithString: "")
    private let addButton = NSButton()
    private var draft: NSTextView!
    private var previewView: NSTextView!
    private var previewScroll: NSScrollView!

    private var latest: Preview?
    private var hues: [String: Int] = [:]
    private var awaiting = 0
    private var debounce: Timer?
    private var monitor: Any?
    private var notice: NSAttributedString?
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
        switch Keys.press(code: event.keyCode,
                          command: event.modifierFlags.contains(.command),
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
        case .pass:
            return false
        }
        return true
    }

    private var canTakeBack: Bool {
        guard let takeBack else { return false }
        return Date().timeIntervalSince(takeBack.at) < Self.secondThoughts
    }

    // ── reading the draft ───────────────────────────────────────────────────

    func textDidChange(_ notification: Notification) {
        // Anything the window was saying is about the draft as it was.
        notice = nil
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

    private func drawPreview() {
        guard let storage = previewView.textStorage else { return }
        let (text, marked) = Render.preview(latest, hues: hues, caret: caretLine)
        storage.setAttributedString(text)
        // Follow the line being typed rather than sitting at the top of a page
        // you typed several minutes ago.
        if let marked { previewView.scrollRangeToVisible(marked) }
    }

    private func drawChrome() {
        summary.attributedStringValue = Render.heading(Render.summary(latest))

        addButton.isEnabled = (latest?.tasks ?? 0) > 0
        addButton.title = latest.map { $0.tasks > 0 ? "file \($0.tasks)" : "file" } ?? "file"

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

    // ── layout ──────────────────────────────────────────────────────────────

    private func build() {
        guard let root = panel.contentView else { return }

        titleField.placeholderString = "name this call (optional)"
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

        addButton.bezelStyle = .rounded
        addButton.title = "file"
        addButton.keyEquivalent = "\r"
        addButton.keyEquivalentModifierMask = [.command]
        addButton.target = self
        addButton.action = #selector(addPressed)

        let keys = NSTextField(labelWithString: "esc file · ⌘⌫ scrap")
        keys.font = Type.mono
        keys.textColor = Theme.shared.colour("ink4")

        let views: [NSView] = [
            titleField, headRule, draftScroll, divider, summary, previewScrollView,
            footRule, hint, keys, addButton,
        ]
        for v in views {
            v.translatesAutoresizingMaskIntoConstraints = false
            root.addSubview(v)
        }

        let pad: CGFloat = 20
        NSLayoutConstraint.activate([
            titleField.topAnchor.constraint(equalTo: root.topAnchor, constant: 16),
            titleField.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad),
            titleField.trailingAnchor.constraint(equalTo: root.trailingAnchor, constant: -pad),

            headRule.topAnchor.constraint(equalTo: titleField.bottomAnchor, constant: 12),
            headRule.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            headRule.trailingAnchor.constraint(equalTo: root.trailingAnchor),

            draftScroll.topAnchor.constraint(equalTo: headRule.bottomAnchor),
            draftScroll.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            draftScroll.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            draftScroll.heightAnchor.constraint(equalTo: root.heightAnchor, multiplier: 0.42),

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

    @objc private func addPressed() { fileAndClose() }

    @objc private func titleCommitted() { panel.makeFirstResponder(draft) }

    /// The one incantation for a text view inside a scroll view, with every
    /// helpful substitution turned off: smart quotes and smart dashes rewrite
    /// the very characters the grammar is made of.
    private static func textView(editable: Bool) -> (NSScrollView, NSTextView) {
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
