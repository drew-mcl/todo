import AppKit
import Carbon.HIToolbox

// A system-wide hotkey, registered the old way.
//
// Carbon's RegisterEventHotKey is the one route to a global shortcut that asks
// the user for nothing: no Accessibility permission, no event tap, no prompt on
// first run. It is ancient and it is still the right answer.

final class Hotkey {
    static let shared = Hotkey()

    /// What each registered combination should do, by the id it was given.
    private var actions: [UInt32: () -> Void] = [:]
    private var refs: [UInt32: EventHotKeyRef] = [:]
    private var labels: [UInt32: String] = [:]
    private var handler: EventHandlerRef?

    /// What a shortcut is, in the form the menu shows it. Empty when it could
    /// not be claimed, so the menu cannot offer a key belonging to something
    /// else.
    func label(_ id: UInt32) -> String { labels[id] ?? "" }

    /// Register the combination described by spec ("opt space", "ctrl+opt+t").
    /// Returns false if it is unreadable or already spoken for by another
    /// application, which is worth saying out loud rather than leaving the user
    /// pressing a key that does nothing.
    @discardableResult
    func register(_ spec: String, id: UInt32, does: @escaping () -> Void) -> Bool {
        unregister(id)
        guard let combo = Combination(spec) else { return false }

        if handler == nil {
            var kind = EventTypeSpec(
                eventClass: OSType(kEventClassKeyboard),
                eventKind: UInt32(kEventHotKeyPressed))
            InstallEventHandler(GetApplicationEventTarget(), { _, event, _ -> OSStatus in
                var pressed = EventHotKeyID()
                GetEventParameter(event, EventParamName(kEventParamDirectObject),
                                  EventParamType(typeEventHotKeyID), nil,
                                  MemoryLayout<EventHotKeyID>.size, nil, &pressed)
                let id = pressed.id
                DispatchQueue.main.async { Hotkey.shared.fire(id) }
                return noErr
            }, 1, &kind, nil, &handler)
        }

        var ref: EventHotKeyRef?
        let hotKeyID = EventHotKeyID(signature: OSType(0x544F444F), id: id) // 'TODO'
        let status = RegisterEventHotKey(
            combo.key, combo.modifiers, hotKeyID, GetApplicationEventTarget(), 0, &ref)
        guard status == noErr, let ref else { return false }

        refs[id] = ref
        labels[id] = combo.label
        actions[id] = does
        return true
    }

    private func fire(_ id: UInt32) { actions[id]?() }

    func unregister(_ id: UInt32) {
        if let ref = refs[id] { UnregisterEventHotKey(ref) }
        refs[id] = nil
        labels[id] = nil
        actions[id] = nil
    }

    func unregisterAll() {
        for id in refs.keys { unregister(id) }
    }

    /// Combination reads "cmd+shift+space" and the like.
    private struct Combination {
        let key: UInt32
        let modifiers: UInt32
        let label: String

        init?(_ spec: String) {
            var mods: UInt32 = 0
            var code: UInt32?
            var shown: [String] = []

            for part in spec.lowercased().split(whereSeparator: { "+- ".contains($0) }) {
                switch String(part) {
                case "cmd", "command", "⌘": mods |= UInt32(cmdKey); shown.append("⌘")
                case "opt", "option", "alt", "⌥": mods |= UInt32(optionKey); shown.append("⌥")
                case "ctrl", "control", "⌃": mods |= UInt32(controlKey); shown.append("⌃")
                case "shift", "⇧": mods |= UInt32(shiftKey); shown.append("⇧")
                default:
                    guard let k = Combination.keys[String(part)] else { return nil }
                    code = k.0
                    shown.append(k.1)
                }
            }
            guard let code, mods != 0 else { return nil }
            self.key = code
            self.modifiers = mods
            self.label = shown.joined()
        }

        /// Enough of the keyboard to name a shortcut with. Virtual key codes are
        /// positional, so these are the codes for a US layout -- which is what
        /// every other application's default shortcut assumes as well.
        static let keys: [String: (UInt32, String)] = [
            "space": (49, "Space"), "return": (36, "↩"), "tab": (48, "⇥"),
            "a": (0, "A"), "b": (11, "B"), "c": (8, "C"), "d": (2, "D"),
            "e": (14, "E"), "f": (3, "F"), "g": (5, "G"), "h": (4, "H"),
            "i": (34, "I"), "j": (38, "J"), "k": (40, "K"), "l": (37, "L"),
            "m": (46, "M"), "n": (45, "N"), "o": (31, "O"), "p": (35, "P"),
            "q": (12, "Q"), "r": (15, "R"), "s": (1, "S"), "t": (17, "T"),
            "u": (32, "U"), "v": (9, "V"), "w": (13, "W"), "x": (7, "X"),
            "y": (16, "Y"), "z": (6, "Z"),
            "1": (18, "1"), "2": (19, "2"), "3": (20, "3"), "4": (21, "4"),
            "5": (23, "5"), "6": (22, "6"), "7": (26, "7"), "8": (28, "8"),
            "9": (25, "9"), "0": (29, "0"),
        ]
    }
}
