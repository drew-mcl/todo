import AppKit
import Carbon.HIToolbox

// A system-wide hotkey, registered the old way.
//
// Carbon's RegisterEventHotKey is the one route to a global shortcut that asks
// the user for nothing: no Accessibility permission, no event tap, no prompt on
// first run. It is ancient and it is still the right answer.

final class Hotkey {
    static let shared = Hotkey()

    var onFire: (() -> Void)?

    private var ref: EventHotKeyRef?
    private var handler: EventHandlerRef?

    /// What the shortcut is, in the form the menu shows it.
    private(set) var label = ""

    /// Register the combination described by spec ("opt space", "ctrl+opt+t").
    /// Returns false if the combination is unreadable or already spoken for by
    /// another application, which is worth saying out loud rather than leaving
    /// the user pressing a key that does nothing.
    @discardableResult
    func register(_ spec: String) -> Bool {
        unregister()
        guard let combo = Combination(spec) else { return false }
        label = combo.label

        if handler == nil {
            var kind = EventTypeSpec(
                eventClass: OSType(kEventClassKeyboard),
                eventKind: UInt32(kEventHotKeyPressed))
            InstallEventHandler(GetApplicationEventTarget(), { _, _, _ -> OSStatus in
                DispatchQueue.main.async { Hotkey.shared.onFire?() }
                return noErr
            }, 1, &kind, nil, &handler)
        }

        let id = EventHotKeyID(signature: OSType(0x544F444F), id: 1) // 'TODO'
        let status = RegisterEventHotKey(
            combo.key, combo.modifiers, id, GetApplicationEventTarget(), 0, &ref)
        return status == noErr
    }

    func unregister() {
        if let ref { UnregisterEventHotKey(ref) }
        ref = nil
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
