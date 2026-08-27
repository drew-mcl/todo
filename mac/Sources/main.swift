import AppKit

// todo, one keystroke away from wherever you are.
//
// An accessory app: no Dock icon, no window until you ask for one, one item in
// the menu bar so it can be found and quit. It holds no list of its own -- the
// window it opens talks to the same `todo` binary and the same SQLite file the
// terminal app and the web one do, so what you type here is on the list before
// the window has finished closing.

let defaultHotkey = "opt space"

final class Bar: NSObject, NSApplicationDelegate {
    private var bridge: Bridge?
    private var capture: CaptureController?
    private var status: NSStatusItem?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        buildMenu()
        buildStatusItem()

        guard let bridge = Bridge() else {
            complain("todo was not found",
                     "The capture bar runs the todo binary. Put it on your PATH, or "
                     + "point at it with: defaults write com.drew-mcl.todo.capture binary /path/to/todo")
            NSApp.terminate(nil)
            return
        }
        self.bridge = bridge
        bridge.onBroken = { [weak self] why in self?.note(why) }
        bridge.start()

        // The colours come from the same place the other two front ends get
        // them, so nothing is drawn until they have arrived.
        bridge.send("hello") { [weak self] reply in
            guard let hello = reply.hello else {
                self?.note(reply.error ?? "todo bridge did not answer")
                return
            }
            Theme.shared.adopt(hello.palette)
            self?.capture?.repaint()
        }

        capture = CaptureController(bridge: bridge)

        let spec = UserDefaults.standard.string(forKey: "hotkey") ?? defaultHotkey
        Hotkey.shared.onFire = { [weak self] in self?.capture?.toggle() }
        if !Hotkey.shared.register(spec) && !Hotkey.shared.register(defaultHotkey) {
            note("that shortcut is taken; open the window from the menu bar")
        }
        refreshMenu()
    }

    /// Opening the app again -- from Finder, from Spotlight, from `open -a` --
    /// is a request for the window, since there is nothing else to look at.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        capture?.show()
        return false
    }

    func applicationWillTerminate(_ notification: Notification) {
        Hotkey.shared.unregister()
        bridge?.stop()
    }

    // ── the menu bar ────────────────────────────────────────────────────────

    private func buildStatusItem() {
        status = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        status?.button?.image = NSImage(
            systemSymbolName: "square.and.pencil", accessibilityDescription: "todo")
        status?.button?.image?.isTemplate = true
        refreshMenu()
    }

    private func refreshMenu() {
        let menu = NSMenu()
        let shortcut = Hotkey.shared.label.isEmpty ? "" : "  " + Hotkey.shared.label
        menu.addItem(withTitle: "Capture" + shortcut, action: #selector(openCapture), keyEquivalent: "")
        menu.addItem(withTitle: "Open todo in a browser", action: #selector(openWeb), keyEquivalent: "")
        menu.addItem(.separator())
        menu.addItem(withTitle: "Quit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        for entry in menu.items where entry.action == #selector(openCapture) || entry.action == #selector(openWeb) {
            entry.target = self
        }
        status?.menu = menu
    }

    @objc private func openCapture() { capture?.show() }

    @objc private func openWeb() {
        guard let binary = Bridge.locate() else { return }
        let serve = Process()
        serve.executableURL = URL(fileURLWithPath: binary)
        serve.arguments = ["serve", "--open"]
        try? serve.run()
    }

    /// An application with no menu bar has no ⌘V, which for a window whose whole
    /// job is taking a paste would be a strange thing to leave out.
    private func buildMenu() {
        let main = NSMenu()

        let appItem = NSMenuItem()
        let app = NSMenu()
        app.addItem(withTitle: "Quit todo capture",
                    action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = app
        main.addItem(appItem)

        let editItem = NSMenuItem()
        let edit = NSMenu(title: "Edit")
        edit.addItem(withTitle: "Undo", action: Selector(("undo:")), keyEquivalent: "z")
        let redo = edit.addItem(withTitle: "Redo", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        edit.addItem(.separator())
        edit.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        edit.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        edit.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        edit.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        editItem.submenu = edit
        main.addItem(editItem)

        NSApp.mainMenu = main
    }

    // ── saying something went wrong ─────────────────────────────────────────

    private func note(_ message: String) {
        status?.button?.toolTip = message
        NSLog("todo capture: %@", message)
    }

    private func complain(_ title: String, _ body: String) {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = body
        alert.alertStyle = .warning
        alert.runModal()
    }
}

let application = NSApplication.shared
let bar = Bar()
application.delegate = bar
application.run()
