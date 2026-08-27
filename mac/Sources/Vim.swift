import Foundation

// Normal mode for the capture box, the same small set the browser answers to.
//
// A pure function of the text, where the caret is and what was pressed, so it
// can be checked without a window. What the keys are comes down the bridge from
// internal/vim; this is one of the two places that has to mean it.

enum VimMode: String {
    case insert
    case normal
}

/// What the box should do besides edit itself.
enum VimExit {
    case file
    case scrap
    case help
}

struct VimState {
    var text: String
    var at: Int
    var mode: VimMode = .insert
    /// A half-typed prefix: the d of dd, the g of gg, the Z of ZZ.
    var pending: String = ""
}

struct VimResult {
    var state: VimState
    /// False when the key was not ours and the text view should have it.
    var handled: Bool
    var exit: VimExit?
}

enum Vim {
    /// press applies one keystroke, in UTF-16 offsets because that is what a
    /// text view counts in.
    static func press(_ key: String, _ s: VimState) -> VimResult {
        var out = s
        let units = Array(s.text.utf16)
        let bounds = line(units, s.at)

        if s.mode == .insert {
            // The one key that means anything while typing. The rest is text.
            guard key == "\u{1b}" else { return VimResult(state: s, handled: false) }
            out.mode = .normal
            out.pending = ""
            // vim leaves the caret on the character it was after.
            out.at = max(bounds.start, s.at - 1)
            return VimResult(state: out, handled: true)
        }

        // A prefix has to be remembered between keystrokes.
        if !s.pending.isEmpty {
            let had = s.pending
            out.pending = ""
            switch (had, key) {
            case ("d", "d"):
                return cut(out, units, bounds.start, min(bounds.end + 1, units.count))
            case ("c", "c"):
                var r = cut(out, units, bounds.start, bounds.end)
                r.state.mode = .insert
                return r
            case ("g", "g"):
                return move(out, 0, units)
            case ("Z", "Z"):
                return VimResult(state: out, handled: true, exit: .file)
            case ("Z", "Q"):
                return VimResult(state: out, handled: true, exit: .scrap)
            default:
                return VimResult(state: out, handled: true) // an abandoned prefix eats its key
            }
        }

        switch key {
        case "\u{1b}", "\r", "\n":
            // Once you have stopped typing, return means the same as it does in
            // every other box on the machine.
            return VimResult(state: out, handled: true, exit: .file)
        case "?":
            return VimResult(state: out, handled: true, exit: .help)

        // Back to typing.
        case "i":
            return typing(out, s.at, units)
        case "a":
            return typing(out, min(s.at + 1, bounds.end), units)
        case "I":
            return typing(out, firstThing(units, bounds), units)
        case "A":
            return typing(out, bounds.end, units)
        case "o":
            return open(out, units, at: bounds.end, caret: bounds.end + 1)
        case "O":
            return open(out, units, at: bounds.start, caret: bounds.start)

        // Moving.
        case "h":
            return move(out, max(bounds.start, s.at - 1), units)
        case "l":
            return move(out, min(bounds.end, s.at + 1), units)
        case "j":
            return move(out, vertical(units, s.at, 1), units)
        case "k":
            return move(out, vertical(units, s.at, -1), units)
        case "0":
            return move(out, bounds.start, units)
        case "^":
            return move(out, firstThing(units, bounds), units)
        case "$":
            return move(out, bounds.end, units)
        case "G":
            return move(out, units.count, units)

        // Words.
        case "w":
            return move(out, nextWord(units, s.at), units)
        case "b":
            return move(out, prevWord(units, s.at), units)
        case "e":
            return move(out, wordEnd(units, s.at), units)

        // Changing.
        case "x":
            guard s.at < bounds.end else { return VimResult(state: out, handled: true) }
            return cut(out, units, s.at, s.at + 1)
        case "D":
            return cut(out, units, s.at, bounds.end)

        // Undo is the text view's own.
        case "u":
            return VimResult(state: out, handled: false)

        // Prefixes.
        case "d", "c", "g", "Z":
            out.pending = key
            return VimResult(state: out, handled: true)

        default:
            // Every other key is swallowed: a stray letter in normal mode must
            // never end up in the draft.
            return VimResult(state: out, handled: true)
        }
    }

    // ── the pieces ──────────────────────────────────────────────────────────

    private struct Bounds {
        let start: Int
        let end: Int
        var column: Int { at - start }
        let at: Int
    }

    private static let newline = UInt16(10)

    private static func line(_ units: [UInt16], _ at: Int) -> Bounds {
        var start = 0
        var i = min(at, units.count) - 1
        while i >= 0 {
            if units[i] == newline {
                start = i + 1
                break
            }
            i -= 1
        }
        var end = units.count
        var j = min(at, units.count)
        while j < units.count {
            if units[j] == newline {
                end = j
                break
            }
            j += 1
        }
        return Bounds(start: start, end: end, at: min(at, units.count))
    }

    private static func isWord(_ u: UInt16) -> Bool {
        let c = Character(UnicodeScalar(u) ?? " ")
        return c.isLetter || c.isNumber || c == "_"
    }

    private static func isSpace(_ u: UInt16) -> Bool {
        u == 32 || u == 9 || u == newline
    }

    private static func firstThing(_ units: [UInt16], _ b: Bounds) -> Int {
        var i = b.start
        while i < b.end && (units[i] == 32 || units[i] == 9) { i += 1 }
        return i
    }

    private static func nextWord(_ units: [UInt16], _ at: Int) -> Int {
        var i = at
        if i < units.count && isWord(units[i]) {
            while i < units.count && isWord(units[i]) { i += 1 }
        } else if i < units.count && !isSpace(units[i]) {
            i += 1
        }
        while i < units.count && isSpace(units[i]) { i += 1 }
        return i
    }

    private static func prevWord(_ units: [UInt16], _ at: Int) -> Int {
        var i = at - 1
        while i > 0 && isSpace(units[i]) { i -= 1 }
        while i > 0 && isWord(units[i - 1]) { i -= 1 }
        return max(0, i)
    }

    private static func wordEnd(_ units: [UInt16], _ at: Int) -> Int {
        var i = at + 1
        while i < units.count && isSpace(units[i]) { i += 1 }
        while i + 1 < units.count && isWord(units[i + 1]) { i += 1 }
        return min(i, max(0, units.count - 1))
    }

    /// Up or down a line, holding the column as vim does.
    private static func vertical(_ units: [UInt16], _ at: Int, _ by: Int) -> Int {
        let here = line(units, at)
        if by < 0 {
            guard here.start > 0 else { return at }
            let above = line(units, here.start - 1)
            return min(above.start + here.column, above.end)
        }
        guard here.end < units.count else { return at }
        let below = line(units, here.end + 1)
        return min(below.start + here.column, below.end)
    }

    private static func move(_ s: VimState, _ to: Int, _ units: [UInt16]) -> VimResult {
        var out = s
        out.at = max(0, min(to, units.count))
        out.pending = ""
        return VimResult(state: out, handled: true)
    }

    private static func typing(_ s: VimState, _ to: Int, _ units: [UInt16]) -> VimResult {
        var r = move(s, to, units)
        r.state.mode = .insert
        return r
    }

    private static func cut(_ s: VimState, _ units: [UInt16], _ from: Int, _ to: Int) -> VimResult {
        var out = s
        var kept = Array(units[0..<max(0, min(from, units.count))])
        kept += Array(units[min(to, units.count)...])
        out.text = String(decoding: kept, as: UTF16.self)
        out.at = min(from, kept.count)
        out.pending = ""
        return VimResult(state: out, handled: true)
    }

    private static func open(_ s: VimState, _ units: [UInt16], at: Int, caret: Int) -> VimResult {
        var out = s
        var made = Array(units[0..<min(at, units.count)])
        made.append(newline)
        made += Array(units[min(at, units.count)...])
        out.text = String(decoding: made, as: UTF16.self)
        out.at = min(caret, made.count)
        out.mode = .insert
        out.pending = ""
        return VimResult(state: out, handled: true)
    }
}
