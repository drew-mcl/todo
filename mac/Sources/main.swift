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

    /// What is wrong, if anything. A hotkey that could not be claimed is
    /// otherwise perfectly silent: you press it, nothing happens, and nothing
    /// anywhere says why.
    private var trouble: String?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        buildMenu()
        buildStatusItem()

        guard let bridge = Bridge() else {
            // No modal. This runs at login, and an alert nobody is sitting in
            // front of blocks the app rather than telling anyone anything --
            // which is how it wedged the first time. The menu bar says it
            // instead, where there is already somewhere to look.
            note("todo was not found — see the menu")
            trouble = "todo was not found; reinstall with: make bar-install"
            refreshMenu()
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
            Theme.shared.adopt(hello.keys ?? [])
            self?.capture?.repaint()
        }

        capture = CaptureController(bridge: bridge)

        let spec = UserDefaults.standard.string(forKey: "hotkey") ?? defaultHotkey
        Hotkey.shared.onFire = { [weak self] in self?.capture?.toggle() }
        if !Hotkey.shared.register(spec) {
            trouble = "\(spec) is taken or unreadable"
            if spec != defaultHotkey && Hotkey.shared.register(defaultHotkey) {
                trouble = "\(spec) is taken — using \(defaultHotkey) instead"
            } else if Hotkey.shared.label.isEmpty {
                trouble = "\(spec) is taken by something else"
            }
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
        if let trouble {
            let said = menu.addItem(withTitle: trouble, action: nil, keyEquivalent: "")
            said.isEnabled = false
            menu.addItem(.separator())
        }
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
        trouble = message
        status?.button?.toolTip = message
        NSLog("todo capture: %@", message)
        refreshMenu()
    }

}

let application = NSApplication.shared
let bar = Bar()
application.delegate = bar
application.run()
