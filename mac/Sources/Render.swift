import AppKit

// Drawing the parse, kept apart from the window that shows it.
//
// Two things here are worth being sure of and neither needs a screen: the spans
// that colour the draft have to line up with the characters actually in it, and
// the preview has to mark the line the caret is on. Both are pure functions of
// what came back down the bridge, so mac/Tests exercises them directly.

/// A page being built.
///
/// Appending to the string and setting attributes over the range is the cheap
/// way round: the obvious one makes an NSAttributedString for every coloured
/// fragment, and a page of blocks is twenty of those each, thrown away on every
/// keystroke.
struct Page {
    let out = NSMutableAttributedString()

    var length: Int { out.length }

    /// Every append to the backing string makes the whole thing re-check its
    /// attributes, which turns building a page into O(page²). Batched, it is
    /// checked once at the end.
    func begin() { out.beginEditing() }
    func end() { out.endEditing() }

    func put(_ text: String, _ font: NSFont, _ colour: NSColor,
             extra: [NSAttributedString.Key: Any] = [:]) {
        guard !text.isEmpty else { return }
        let start = out.length
        out.mutableString.append(text)
        var attributes: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: colour]
        attributes.merge(extra) { _, new in new }
        out.setAttributes(attributes, range: NSRange(location: start, length: out.length - start))
    }

    /// A run that is always the same is built once and appended, which is ten
    /// times cheaper than writing the text and then saying what colour it is.
    func append(_ ready: NSAttributedString) { out.append(ready) }

    /// Lay a paragraph style over everything written since `from`, which is how
    /// a block gets its gap without every fragment carrying one.
    func space(from: Int, _ style: NSParagraphStyle) {
        guard out.length > from else { return }
        out.addAttribute(.paragraphStyle, value: style,
                         range: NSRange(location: from, length: out.length - from))
    }
}

enum Render {
    /// Where each highlighted span falls in text.
    ///
    /// The parser rewrites a look-alike bar into a real one, so a line's spans
    /// only line up when nothing needed rewriting. A line that does not fit is
    /// left out rather than mispainted -- plain ink is a smaller lie than the
    /// wrong colours in the wrong places.
    static func spans(for text: String, lines: [PreviewLine]) -> [(NSRange, String)] {
        let ns = text as NSString
        var starts: [Int] = [0]
        ns.enumerateSubstrings(in: NSRange(location: 0, length: ns.length),
                               options: [.byLines, .substringNotRequired]) { _, _, enclosing, _ in
            starts.append(enclosing.location + enclosing.length)
        }

        var out: [(NSRange, String)] = []
        for line in lines {
            guard let tokens = line.tokens, line.n >= 1, line.n - 1 < starts.count else { continue }
            let start = starts[line.n - 1]
            let width = tokens.reduce(0) { $0 + ($1.text as NSString).length }
            guard start + width <= ns.length else { continue }

            var at = start
            for token in tokens {
                let length = (token.text as NSString).length
                if length > 0 {
                    out.append((NSRange(location: at, length: length), token.kind))
                }
                at += length
            }
        }
        return out
    }

    /// The preview, and the range of the block the caret's line produced --
    /// which is what the window scrolls to, so the preview follows the draft.
    ///
    /// Every block is the same shape: what the line will be called, then the
    /// quiet detail underneath it, then a gap. They shared a line and three
    /// different left edges before, which put a 13pt face and an 11pt one on
    /// one baseline and read as a jumble rather than a list.
    /// `settled` is whether typing has paused. Until it has, the line under the
    /// caret is not told off: half a line is not yet a mistake, and a complaint
    /// that appears before you have finished the word is only noise.
    static func preview(_ preview: Preview?, hues: [String: Int], caret: Int,
                        settled: Bool = true) -> (NSAttributedString, [Int: NSRange]) {
        guard let preview, !preview.lines.isEmpty else {
            return (quiet("your lines appear here as they will be filed", "ink4"), [:])
        }

        let page = Page()
        page.begin()
        defer { page.end() }
        let ink = Theme.shared.colour("ink")
        let ink3 = Theme.shared.colour("ink3")
        let ink4 = Theme.shared.colour("ink4")
        let danger = Theme.shared.colour("danger")
        var blocks: [Int: NSRange] = [:]

        for line in preview.lines {
            let start = page.length
            // Still being written, and not finished being wrong.
            let hush = line.n == caret && !settled
            let live = line.n == caret

            /// The bar runs down the whole block, one row at a time, so it marks
            /// the line being typed the way the terminal's does. It is always
            /// drawn and sometimes invisible, so moving it is a change of colour
            /// rather than a change of text.
            // A paragraph takes its style from its first character, so the gap
            // has to be laid over the whole of the last row, not its newline.
            var lastRow = page.length
            func row(_ body: () -> Void) {
                lastRow = page.length
                page.append(live ? barLit : barDark)
                body()
                page.append(newline)
            }

            switch line.kind {
            case "task":
                guard let task = line.task else { continue }
                row { page.put(task.title, Type.strong, ink) }
                row { meta(page, task, hue: hues[task.topic]) }
                if !task.note.isEmpty {
                    row {
                        page.put("↳ " + task.note.replacingOccurrences(of: "\n", with: " "),
                                 Type.mono, ink3)
                    }
                }
                if !task.warning.isEmpty && !hush {
                    row { page.put(task.warning, Type.mono, danger) }
                }

            case "note":
                // It is already shown under the task it belongs to. Printing it
                // again, with its own arrow, only made the list look like it had
                // read the line twice.
                continue

            default:
                // A line with nothing on it yet is not worth a block at all.
                if hush && line.raw.trimmingCharacters(in: .whitespaces).isEmpty { continue }
                row {
                    page.put(line.raw, Type.mono, ink4, extra: [
                        .strikethroughStyle: NSUnderlineStyle.single.rawValue,
                        .strikethroughColor: Theme.shared.colour("line"),
                    ])
                }
                if !hush { row { page.put(line.reason ?? "skipped", Type.mono, ink4) } }
            }

            // Once over the block, then the gap over its last row, rather than
            // a style laid over every row in it.
            page.space(from: start, inside)
            page.space(from: lastRow, between)
            blocks[line.n] = NSRange(location: start, length: page.length - start)
        }
        return (page.out, blocks)
    }

    /// The gap that separates one line's block from the next.
    ///
    /// Made once. Two paragraph styles per row, times a page of rows, times
    /// every keystroke, is a lot of objects to build and throw away to say the
    /// same two things each time.
    private static let inside = paragraph(spacing: 0, indent: 20)
    private static let between = paragraph(spacing: 10, indent: 20)
    private static let dayInside = paragraph(spacing: 0, indent: 22)
    private static let dayBetween = paragraph(spacing: 9, indent: 22)

    /// The pieces written on nearly every row, built once each.
    private static let newline = NSAttributedString(string: "\n")
    private static let dot = NSAttributedString(string: " · ", attributes: [
        .font: Type.mono, .foregroundColor: Theme.shared.colour("ink4"),
    ])
    private static let barLit = NSAttributedString(string: "▏ ", attributes: [
        .font: Type.mono, .foregroundColor: Theme.shared.colour("accent"),
    ])
    private static let barDark = NSAttributedString(string: "▏ ", attributes: [
        .font: Type.mono, .foregroundColor: NSColor.clear,
    ])

    private static func paragraph(spacing: CGFloat, indent: CGFloat) -> NSParagraphStyle {
        let style = NSMutableParagraphStyle()
        style.lineSpacing = 1
        style.paragraphSpacing = spacing
        style.headIndent = indent
        return style
    }

    /// What the draft currently amounts to, in the same words the terminal uses.
    static func summary(_ preview: Preview?) -> String {
        guard let preview else { return "NOTHING YET" }
        var parts: [String] = []
        if preview.tasks > 0 { parts.append("\(preview.tasks) TASK\(preview.tasks == 1 ? "" : "S")") }
        if preview.notes > 0 { parts.append("\(preview.notes) NOTE\(preview.notes == 1 ? "" : "S")") }
        if preview.skipped > 0 { parts.append("\(preview.skipped) SKIPPED") }
        return parts.isEmpty ? "NOTHING YET" : parts.joined(separator: " · ")
    }

    /// The grammar, in the colours it will be read in.
    static func grammar() -> NSAttributedString {
        let out = NSMutableAttributedString()
        let spelling: [(String, String)] = [
            ("topic", "topic"), (" | ", "pipe"), ("what needs doing", "text"),
            (" | ", "pipe"), ("today", "due"), (" ", "text"), ("@who", "who"),
            (" ", "text"), ("!!", "pri"), (" ", "text"), ("#tag", "tag"),
            ("  > note", "note"),
        ]
        for (text, kind) in spelling {
            out.append(NSAttributedString(string: text, attributes: [
                .font: Type.mono, .foregroundColor: Theme.shared.token(kind),
            ]))
        }
        return out
    }

    /// The keys the capture box answers to, as the bridge described them.
    static func sheet(_ groups: [KeyGroup]) -> NSAttributedString {
        let out = NSMutableAttributedString()
        if groups.isEmpty {
            return quiet("the keys have not arrived from todo bridge yet", "ink4")
        }
        for group in groups {
            out.append(heading(group.name.uppercased()))
            out.append(NSAttributedString(string: "\n"))
            for key in group.keys {
                out.append(NSAttributedString(string: pad(key.press, to: 12), attributes: [
                    .font: NSFont.monospacedSystemFont(ofSize: 12, weight: .semibold),
                    .foregroundColor: Theme.shared.colour("accent"),
                ]))
                out.append(quiet(key.does, "ink3"))
                out.append(NSAttributedString(string: "\n"))
            }
            out.append(NSAttributedString(string: "\n"))
        }
        out.append(quiet("esc stops typing · esc again files what is there · any key closes this",
                         "ink4"))
        return out
    }

    private static func pad(_ s: String, to n: Int) -> String {
        s.count >= n ? s + "  " : s + String(repeating: " ", count: n - s.count)
    }

    // ── one row ─────────────────────────────────────────────────────────────

    /// The quiet detail under a title: whose it is, when, and how loud.
    private static func meta(_ page: Page, _ task: PreviewTask, hue: Int?) {
        let ink3 = Theme.shared.colour("ink3")
        var first = true
        func part(_ body: () -> Void) {
            if !first { page.append(dot) }
            first = false
            body()
        }

        part {
            page.put("● ", Type.mono, Theme.shared.topicColour(hue))
            page.put(task.topic, Type.mono, ink3)
        }
        if !task.dueLabel.isEmpty {
            part { page.put(task.dueLabel.lowercased(), Type.mono, Theme.shared.colour("accent")) }
        }
        if !task.assignee.isEmpty { part { page.put(task.assignee, Type.mono, ink3) } }
        if task.priority > 0 {
            part {
                page.put(String(repeating: "!", count: task.priority), Type.mono,
                         Theme.shared.colour("danger"))
            }
        }
        for tag in task.tags { part { page.put("#" + tag, Type.mono, ink3) } }
    }

    // ── the lists ───────────────────────────────────────────────────────────

    /// A list: its sections, one task to a block, the same shape the preview
    /// uses so the two windows read as one app.
    static func day(_ day: Day, cursor: Int) -> (NSAttributedString, NSRange?) {
        let page = Page()
        page.begin()
        defer { page.end() }
        let ink = Theme.shared.colour("ink")
        let ink3 = Theme.shared.colour("ink3")
        let ink4 = Theme.shared.colour("ink4")
        let accent = Theme.shared.colour("accent")
        let danger = Theme.shared.colour("danger")
        var marked: NSRange?
        var index = 0

        for section in day.sections {
            if !section.label.isEmpty {
                let top = page.length
                page.put("  " + section.label.uppercased(), Type.heading,
                         section.late ? danger : ink4, extra: [.kern: 0.8])
                page.append(newline)
                // A heading hugs what it labels; the air belongs above it, and
                // the block before it already leaves some.
                page.space(from: top, dayInside)
            }

            for task in section.tasks {
                let live = index == cursor
                let start = page.length

                var lastRow = page.length
                func row(_ body: () -> Void) {
                    lastRow = page.length
                    page.append(live ? barLit : barDark)
                    body()
                    page.append(newline)
                }

                row { page.put("○ " + task.title, Type.strong, ink) }
                row {
                    page.put("  ● ", Type.mono, Theme.shared.topicColour(day.hues[task.topic]))
                    page.put(task.topic, Type.mono, ink3)
                    if !task.dueLabel.isEmpty {
                        page.append(dot)
                        page.put(task.dueLabel.lowercased(), Type.mono,
                                 section.late ? danger : accent)
                    }
                    if !task.assignee.isEmpty {
                        page.append(dot)
                        page.put(task.assignee, Type.mono, ink3)
                    }
                    if task.priority > 0 {
                        page.append(dot)
                        page.put(String(repeating: "!", count: task.priority), Type.mono, danger)
                    }
                }
                page.space(from: start, dayInside)
                page.space(from: lastRow, dayBetween)

                if live && marked == nil {
                    marked = NSRange(location: start, length: page.length - start)
                }
                index += 1
            }
        }

        if page.length == 0 {
            return (quiet(empty(day), "ink3"), nil)
        }
        return (page.out, marked)
    }

    /// What an empty list should say for itself, which is not the same thing in
    /// each of them: a day with nothing left is the best thing that happens.
    private static func empty(_ day: Day) -> String {
        switch day.view {
        case "today":
            return day.done > 0
                ? "that is today done. \(day.done) closed."
                : "nothing due today."
        case "week":
            return "nothing scheduled this week."
        case "logbook":
            return "nothing finished yet."
        default:
            return "nothing open. press n to write something down."
        }
    }

    /// A section label: the small, spaced capitals the other two front ends use.
    static func heading(_ s: String) -> NSAttributedString {
        NSAttributedString(string: s, attributes: [
            .font: Type.heading, .foregroundColor: Theme.shared.colour("ink4"), .kern: 0.8,
        ])
    }

    private static func quiet(_ s: String, _ colour: String) -> NSAttributedString {
        NSAttributedString(string: s, attributes: [
            .font: Type.mono, .foregroundColor: Theme.shared.colour(colour),
        ])
    }

}
