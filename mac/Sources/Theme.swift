import AppKit

// Every colour in this window came down the bridge from internal/palette. There
// is no second copy of the scheme on this side, which is the only way a topic
// can be the same dot here as it is in the terminal and the browser.

final class Theme {
    static let shared = Theme()

    private var topic: [NSColor] = []
    private var scheme: [String: NSColor] = [:]

    var ready: Bool { !topic.isEmpty }

    /// The keys the capture box answers to, as the bridge described them. The
    /// window shows the list it was handed rather than one of its own.
    private(set) var keys: [KeyGroup] = []

    func adopt(_ keys: [KeyGroup]) { self.keys = keys }

    func adopt(_ palette: Palette) {
        topic = palette.topic.map(Theme.adaptive)
        scheme = palette.scheme.mapValues(Theme.adaptive)
    }

    /// Named colours, with a sane stand-in for the moment before hello lands.
    func colour(_ name: String) -> NSColor {
        if let c = scheme[name] { return c }
        switch name {
        case "ink": return .labelColor
        case "ink2": return .labelColor
        case "ink3": return .secondaryLabelColor
        case "ink4": return .tertiaryLabelColor
        case "line": return .separatorColor
        case "sunk": return .underPageBackgroundColor
        case "accent": return .controlAccentColor
        case "danger": return .systemRed
        default: return .labelColor
        }
    }

    /// The dot a topic gets. The hue is assigned on the other side, where the
    /// whole screenful is known and no two topics can end up sharing one.
    func topicColour(_ hue: Int?) -> NSColor {
        guard let hue, topic.indices.contains(hue) else { return colour("ink3") }
        return topic[hue]
    }

    /// The colour a highlighted span of shorthand is drawn in. Same tokens, same
    /// meanings, as the capture box in the terminal.
    func token(_ kind: String) -> NSColor {
        switch kind {
        case "topic": return colour("ink")
        case "pipe": return colour("ink4")
        case "due": return colour("accent")
        case "who": return topicColour(0)
        case "pri": return colour("danger")
        case "tag": return topicColour(2)
        case "note": return colour("ink3")
        default: return colour("ink2")
        }
    }

    private static func adaptive(_ c: Colour) -> NSColor {
        NSColor(name: nil) { appearance in
            let dark = appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            return NSColor(hex: dark ? c.dark : c.light) ?? .labelColor
        }
    }
}

extension NSColor {
    /// #RRGGBB, which is the only form the palette uses.
    convenience init?(hex: String) {
        var digits = hex
        if digits.hasPrefix("#") { digits.removeFirst() }
        guard digits.count == 6, let value = UInt32(digits, radix: 16) else { return nil }
        self.init(
            srgbRed: CGFloat((value >> 16) & 0xFF) / 255,
            green: CGFloat((value >> 8) & 0xFF) / 255,
            blue: CGFloat(value & 0xFF) / 255,
            alpha: 1)
    }
}

/// The type in this window: monospaced where the grammar matters, because the
/// pipes are meant to line up down the page.
enum Type {
    static let draft = NSFont.monospacedSystemFont(ofSize: 14, weight: .regular)
    static let mono = NSFont.monospacedSystemFont(ofSize: 11.5, weight: .regular)
    static let title = NSFont.systemFont(ofSize: 13.5)
    static let heading = NSFont.monospacedSystemFont(ofSize: 10, weight: .semibold)
}
