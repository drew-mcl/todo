import AppKit

// Drawing the parse, kept apart from the window that shows it.
//
// Two things here are worth being sure of and neither needs a screen: the spans
// that colour the draft have to line up with the characters actually in it, and
// the preview has to mark the line the caret is on. Both are pure functions of
// what came back down the bridge, so mac/Tests exercises them directly.

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
                        settled: Bool = true) -> (NSAttributedString, NSRange?) {
        guard let preview, !preview.lines.isEmpty else {
            return (quiet("your lines appear here as they will be filed", "ink4"), nil)
        }

        let out = NSMutableAttributedString()
        var marked: NSRange?

        for line in preview.lines {
            let start = out.length
            var rows: [NSAttributedString] = []
            // Still being written, and not finished being wrong.
            let hush = line.n == caret && !settled

            switch line.kind {
            case "task":
                if let task = line.task {
                    rows.append(strong(task.title))
                    rows.append(meta(task, hue: hues[task.topic]))
                    if !task.note.isEmpty {
                        rows.append(quiet("↳ " + task.note.replacingOccurrences(of: "\n", with: " "),
                                          "ink3"))
                    }
                    if !task.warning.isEmpty && !hush {
                        rows.append(NSAttributedString(string: task.warning, attributes: [
                            .font: Type.mono, .foregroundColor: Theme.shared.colour("danger"),
                        ]))
                    }
                }
            case "note":
                // It is already shown under the task it belongs to. Printing it
                // again, with its own arrow, only made the list look like it had
                // read the line twice.
                continue
            default:
                // A line with nothing on it yet is not worth a block at all.
                if hush && line.raw.trimmingCharacters(in: .whitespaces).isEmpty { continue }
                rows.append(struck(line.raw))
                if !hush { rows.append(quiet(line.reason ?? "skipped", "ink4")) }
            }

            let live = line.n == caret
            for (i, row) in rows.enumerated() {
                // The bar runs down the whole block, one row at a time, so it
                // marks the line being typed the way the terminal's does.
                let piece = NSMutableAttributedString(string: live ? "▏ " : "  ", attributes: [
                    .font: Type.mono,
                    .foregroundColor: Theme.shared.colour(live ? "accent" : "sunk"),
                ])
                piece.append(row)
                piece.addAttribute(
                    .paragraphStyle, value: paragraph(last: i == rows.count - 1),
                    range: NSRange(location: 0, length: piece.length))
                out.append(piece)
                out.append(NSAttributedString(string: "\n"))
            }

            if live { marked = NSRange(location: start, length: out.length - start) }
        }
        return (out, marked)
    }

    /// The gap that separates one line's block from the next.
    private static func paragraph(last: Bool) -> NSParagraphStyle {
        let style = NSMutableParagraphStyle()
        style.lineSpacing = 1
        style.paragraphSpacing = last ? 10 : 0
        style.headIndent = 20
        return style
    }

    /// What a line will be called.
    private static func strong(_ s: String) -> NSAttributedString {
        NSAttributedString(string: s, attributes: [
            .font: NSFont.monospacedSystemFont(ofSize: 12.5, weight: .medium),
            .foregroundColor: Theme.shared.colour("ink"),
        ])
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
    private static func meta(_ task: PreviewTask, hue: Int?) -> NSAttributedString {
        let out = NSMutableAttributedString()
        var parts: [NSAttributedString] = []

        let dot = NSMutableAttributedString(string: "● ", attributes: [
            .font: Type.mono, .foregroundColor: Theme.shared.topicColour(hue),
        ])
        dot.append(quiet(task.topic, "ink3"))
        parts.append(dot)

        if !task.dueLabel.isEmpty {
            parts.append(NSAttributedString(string: task.dueLabel.lowercased(), attributes: [
                .font: Type.mono, .foregroundColor: Theme.shared.colour("accent"),
            ]))
        }
        if !task.assignee.isEmpty { parts.append(quiet(task.assignee, "ink3")) }
        if task.priority > 0 {
            parts.append(NSAttributedString(
                string: String(repeating: "!", count: task.priority),
                attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("danger")]))
        }
        for tag in task.tags { parts.append(quiet("#" + tag, "ink3")) }

        for (i, part) in parts.enumerated() {
            if i > 0 { out.append(quiet(" · ", "ink4")) }
            out.append(part)
        }
        return out
    }

    /// Where the cursor's row landed, so the list can be scrolled to it.
    private(set) static var dayCursorRange: NSRange?

    /// A list: its sections, one task to a block, the same shape the preview
    /// uses so the two windows read as one app.
    static func day(_ day: Day, cursor: Int) -> NSAttributedString {
        let out = NSMutableAttributedString()
        dayCursorRange = nil
        var index = 0

        for section in day.sections {
            if !section.label.isEmpty {
                out.append(NSAttributedString(string: "  "))
                out.append(NSAttributedString(string: section.label.uppercased(), attributes: [
                    .font: Type.heading,
                    .foregroundColor: Theme.shared.colour(section.late ? "danger" : "ink4"),
                    .kern: 0.8,
                    .paragraphStyle: dayParagraph(last: true),
                ]))
                out.append(NSAttributedString(string: "\n"))
            }

            for task in section.tasks {
                let live = index == cursor
                let rows = [title(task),
                            line(task, hue: day.hues[task.topic], late: section.late)]
                for (i, row) in rows.enumerated() {
                    let piece = NSMutableAttributedString(
                        string: live ? "▏ " : "  ",
                        attributes: [
                            .font: Type.mono,
                            .foregroundColor: Theme.shared.colour(live ? "accent" : "sunk"),
                        ])
                    piece.append(row)
                    piece.addAttribute(.paragraphStyle, value: dayParagraph(last: i == 1),
                                       range: NSRange(location: 0, length: piece.length))
                    if live && dayCursorRange == nil {
                        dayCursorRange = NSRange(location: out.length, length: piece.length)
                    }
                    out.append(piece)
                    out.append(NSAttributedString(string: "\n"))
                }
                index += 1
            }
        }

        if out.length == 0 {
            return quiet(empty(day), "ink3")
        }
        return out
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

    private static func title(_ task: Task) -> NSAttributedString {
        NSAttributedString(string: "○ " + task.title, attributes: [
            .font: NSFont.monospacedSystemFont(ofSize: 12.5, weight: .medium),
            .foregroundColor: Theme.shared.colour("ink"),
        ])
    }

    private static func line(_ task: Task, hue: Int?, late: Bool) -> NSAttributedString {
        let out = NSMutableAttributedString(string: "  ● ", attributes: [
            .font: Type.mono, .foregroundColor: Theme.shared.topicColour(hue),
        ])
        out.append(quiet(task.topic, "ink3"))
        if !task.dueLabel.isEmpty {
            out.append(quiet(" · ", "ink4"))
            out.append(NSAttributedString(string: task.dueLabel.lowercased(), attributes: [
                .font: Type.mono,
                .foregroundColor: Theme.shared.colour(late ? "danger" : "accent"),
            ]))
        }
        if !task.assignee.isEmpty {
            out.append(quiet(" · ", "ink4"))
            out.append(quiet(task.assignee, "ink3"))
        }
        if task.priority > 0 {
            out.append(quiet(" · ", "ink4"))
            out.append(NSAttributedString(
                string: String(repeating: "!", count: task.priority),
                attributes: [.font: Type.mono, .foregroundColor: Theme.shared.colour("danger")]))
        }
        return out
    }

    private static func dayParagraph(last: Bool) -> NSParagraphStyle {
        let style = NSMutableParagraphStyle()
        style.lineSpacing = 1
        style.paragraphSpacing = last ? 9 : 0
        style.headIndent = 22
        return style
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

    private static func struck(_ s: String) -> NSAttributedString {
        NSAttributedString(string: s, attributes: [
            .font: Type.mono,
            .foregroundColor: Theme.shared.colour("ink4"),
            .strikethroughStyle: NSUnderlineStyle.single.rawValue,
            .strikethroughColor: Theme.shared.colour("line"),
        ])
    }
}
