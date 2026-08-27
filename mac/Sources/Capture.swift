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
    private var closeTimer: Timer?
    private var lastBatch: Int?
    private var monitor: Any?
    private var previousApp: NSRunningApplication?

    /// The character range of the row the caret's line produced, so the preview
    /// can be scrolled to it the way the terminal scrolls to it.
    private var markedRange: NSRange?

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
    }

    func hide() {
        closeTimer?.invalidate()
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
    private func handle(_ event: NSEvent) -> Bool {
        guard panel.isKeyWindow else { return false }
        let command = event.modifierFlags.contains(.command)

        switch event.keyCode {
        case 53: // esc
            hide()
            return true
        case 36 where command: // ⌘ return
            commit()
            return true
        case 6 where command && lastBatch != nil: // ⌘z, while there is a capture to take back
            undo()
            return true
        default:
            return false
        }
    }

    // ── reading the draft ───────────────────────────────────────────────────

    func textDidChange(_ notification: Notification) {
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

    private func commit() {
        guard let latest, latest.tasks > 0 else {
            flash("nothing here contains a '|', so nothing would be filed")
            return
        }
        bridge.send("capture", draft: draft.string, title: titleField.stringValue) { [weak self] reply in
            guard let self else { return }
            if let error = reply.error {
                self.flash(error)
                return
            }
            guard let added = reply.added else { return }
            self.lastBatch = added.batchId
            self.draft.string = ""
            self.titleField.stringValue = ""
            self.latest = nil
            self.render()
            self.confirm(added)
        }
    }

    private func undo() {
        guard let batch = lastBatch else { return }
        bridge.send("undo", batch: batch) { [weak self] reply in
            guard let self else { return }
            self.lastBatch = nil
            self.closeTimer?.invalidate()
            self.flash(reply.error ?? "put back \(reply.undone ?? 0)")
        }
    }

    /// The window says what it did and then gets out of the way. Long enough to
    /// read, and long enough to change your mind.
    private func confirm(_ added: Added) {
        let word = added.added == 1 ? "task" : "tasks"
        summary.attributedStringValue = Render.heading("ADDED \(added.added) \(word.uppercased())")
        hint.attributedStringValue = NSAttributedString(
            string: "\(added.today) due today · ⌘Z to take it back",
            attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("ink3")])

        closeTimer?.invalidate()
        closeTimer = Timer.scheduledTimer(withTimeInterval: 1.8, repeats: false) { [weak self] _ in
            self?.hide()
        }
    }

    private func flash(_ message: String) {
        hint.attributedStringValue = NSAttributedString(
            string: message,
            attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("danger")])
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
        addButton.title = latest.map { $0.tasks > 0 ? "add \($0.tasks)" : "add" } ?? "add"

        // A task line typed into the title box is the one mistake this shape of
        // window invites, so it is named rather than left to look like a parser
        // that has stopped reading pipes.
        if titleField.stringValue.contains("|") {
            flash("that is a task line, not a title — it belongs below")
        } else {
            hint.attributedStringValue = Render.grammar()
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
        addButton.title = "add"
        addButton.keyEquivalent = "\r"
        addButton.keyEquivalentModifierMask = [.command]
        addButton.target = self
        addButton.action = #selector(addPressed)

        let keys = NSTextField(labelWithString: "⌘↵ add · esc close")
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

    @objc private func addPressed() { commit() }

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
