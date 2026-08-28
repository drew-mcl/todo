import AppKit

// The lists, over whatever you were doing.
//
// The same panel as the capture box and the same colours, because it is the
// same app arriving the same way -- you press something and it is in front of
// you without a Space to travel to or a window to find.
//
// The keys are the terminal app's, because these are the same four lists: t
// today, w the week, a everything open, l the logbook, n back to the box. One
// vocabulary for one set of lists; a second would be one too many.
//
// It holds nothing of its own. `todo bridge` says what is in each, and closing
// one sends it straight back -- the terminal and the browser see it at once,
// because there is only ever one database.

final class ListController: NSObject, NSWindowDelegate {
    let panel = CapturePanel(size: NSSize(width: 560, height: 440))

    private let bridge: Bridge
    private let heading = NSTextField(labelWithString: "")
    private let meter = NSTextField(labelWithString: "")
    private let keys = NSTextField(labelWithString: "")
    private var list: NSTextView!
    private var listScroll: NSScrollView!

    private var day: Day?
    private var view = "today"
    private var cursor = 0
    private var monitor: Any?
    private var previousApp: NSRunningApplication?

    init(bridge: Bridge) {
        self.bridge = bridge
        super.init()
        build()
        panel.delegate = self
    }

    // ── the window ──────────────────────────────────────────────────────────

    func toggle() { panel.isVisible && panel.isKeyWindow ? hide() : show() }

    func show(_ which: String = "today") {
        if which != view {
            view = which
            cursor = 0
            day = nil
        }
        appear()
    }

    private func appear() {
        previousApp = NSWorkspace.shared.frontmostApplication
        if !panel.isVisible { panel.positionOnActiveScreen() }
        panel.level = .floating
        NSApp.activate(ignoringOtherApps: true)
        panel.makeKeyAndOrderFront(nil)
        installMonitor()
        reload()
    }

    func hide() {
        removeMonitor()
        panel.orderOut(nil)
        previousApp?.activate()
        previousApp = nil
    }

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

    /// The terminal app's keys, because these are the terminal app's lists.
    private func handle(_ event: NSEvent) -> Bool {
        guard panel.isKeyWindow, !event.modifierFlags.contains(.command) else { return false }
        let all = every()

        switch event.charactersIgnoringModifiers {
        case "\u{1b}", "q":
            hide()
        case "j", String(UnicodeScalar(NSDownArrowFunctionKey)!):
            cursor = min(cursor + 1, max(0, all.count - 1))
            draw()
        case "k", String(UnicodeScalar(NSUpArrowFunctionKey)!):
            cursor = max(cursor - 1, 0)
            draw()
        case "g":
            cursor = 0
            draw()
        case "G":
            cursor = max(0, all.count - 1)
            draw()
        case "x", " ", "\r":
            guard cursor < all.count else { return true }
            close(all[cursor])

        // The other lists, one letter each.
        case "t": show("today")
        case "w": show("week")
        case "a": show("all")
        case "l": show("logbook")

        case "n":
            // Back into the box, which is the other half of this.
            hide()
            NotificationCenter.default.post(name: .todoCapture, object: nil)
        default:
            return false
        }
        return true
    }

    // ── the day ─────────────────────────────────────────────────────────────

    private func every() -> [Task] { (day?.sections ?? []).flatMap(\.tasks) }

    private func reload() {
        bridge.send("list", view: view) { [weak self] reply in
            guard let self else { return }
            guard let day = reply.day else {
                self.heading.stringValue = reply.error ?? "todo bridge did not answer"
                return
            }
            self.day = day
            self.cursor = min(self.cursor, max(0, self.every().count - 1))
            self.draw()
        }
    }

    /// Closing one out. The reply carries the day back, so the list redraws
    /// from what is actually stored rather than from a guess.
    private func close(_ task: Task) {
        bridge.send("toggle", task: task.id, view: view) { [weak self] reply in
            guard let self else { return }
            guard let day = reply.day else { return }
            self.day = day
            self.cursor = min(self.cursor, max(0, self.every().count - 1))
            self.draw()
        }
    }

    private func draw() {
        guard let day else { return }
        heading.attributedStringValue = NSAttributedString(
            string: day.label,
            attributes: [.font: Type.title, .foregroundColor: Theme.shared.colour("ink")])

        // Today counts what is left of the day; the other lists just say how
        // many, because "3 of 40 done" is not a fact about anything.
        let total = every().count
        meter.attributedStringValue = Render.heading(
            day.view == "today"
                ? (day.open + day.done == 0
                    ? "NOTHING DUE"
                    : "\(day.done) OF \(day.open + day.done) DONE")
                : (total == 1 ? "1 TASK" : "\(total) TASKS"))

        list.textStorage?.setAttributedString(
            Render.day(day, cursor: cursor))
        if let marked = Render.dayCursorRange { list.scrollRangeToVisible(marked) }
    }

    // ── layout ──────────────────────────────────────────────────────────────

    private func build() {
        guard let root = panel.contentView else { return }

        let (scroll, text) = CaptureController.textView(editable: false)
        list = text
        listScroll = scroll
        list.textContainerInset = NSSize(width: 18, height: 12)

        let headRule = NSBox()
        headRule.boxType = .separator
        let footRule = NSBox()
        footRule.boxType = .separator

        keys.attributedStringValue = NSAttributedString(
            string: "t today · w week · a all · l logbook · n new · x done · esc close",
            attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("ink4")])

        let views: [NSView] = [heading, meter, headRule, scroll, footRule, keys]
        for v in views {
            v.translatesAutoresizingMaskIntoConstraints = false
            root.addSubview(v)
        }

        let pad: CGFloat = 20
        NSLayoutConstraint.activate([
            heading.topAnchor.constraint(equalTo: root.topAnchor, constant: 16),
            heading.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad),

            meter.centerYAnchor.constraint(equalTo: heading.centerYAnchor),
            meter.trailingAnchor.constraint(equalTo: root.trailingAnchor, constant: -pad),
            meter.leadingAnchor.constraint(greaterThanOrEqualTo: heading.trailingAnchor, constant: 12),

            headRule.topAnchor.constraint(equalTo: heading.bottomAnchor, constant: 12),
            headRule.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            headRule.trailingAnchor.constraint(equalTo: root.trailingAnchor),

            scroll.topAnchor.constraint(equalTo: headRule.bottomAnchor),
            scroll.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            scroll.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            scroll.bottomAnchor.constraint(equalTo: footRule.topAnchor),

            footRule.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            footRule.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            footRule.bottomAnchor.constraint(equalTo: keys.topAnchor, constant: -10),

            keys.leadingAnchor.constraint(equalTo: root.leadingAnchor, constant: pad),
            keys.bottomAnchor.constraint(equalTo: root.bottomAnchor, constant: -14),
        ])
    }
}

extension Notification.Name {
    /// The two windows hand over to each other rather than each knowing how to
    /// build the other.
    static let todoCapture = Notification.Name("todo.capture")
    static let todoToday = Notification.Name("todo.today")
}
