import AppKit

// What the capture bar can be checked without a screen.
//
// The window itself has to be looked at, but the two things most likely to be
// quietly wrong do not: the spans that colour the draft have to land on the
// characters actually in it, and the preview has to say the same thing the
// terminal says about the same draft. Both are exercised against a live
// `todo bridge`, so this fails if the wire shapes drift.
//
//   make test-bar

var failures: [String] = []

func check(_ what: String, _ ok: Bool, _ detail: @autoclosure () -> String = "") {
    if ok {
        print("  ok    \(what)")
    } else {
        let extra = detail()
        failures.append(what + (extra.isEmpty ? "" : " — " + extra))
        print("  FAIL  \(what)\(extra.isEmpty ? "" : " — " + extra)")
    }
}

func finish() -> Never {
    print("")
    if failures.isEmpty {
        print("the capture bar agrees with the bridge")
        exit(0)
    }
    for f in failures { print("failed: \(f)") }
    exit(1)
}

let draft = """
prod issue | chase the vendor about the patch | today @sam !!
> they have missed two dates now
| write the postmortem | eow #board
finance want the numbers before friday
"""

guard let bridge = Bridge() else {
    print("could not find the todo binary; build it first with `make`")
    exit(1)
}
bridge.onBroken = { why in
    print("bridge broke: \(why)")
    exit(1)
}
bridge.start()

bridge.send("hello") { reply in
    guard let hello = reply.hello else {
        check("hello answers", false, reply.error ?? "no reply")
        finish()
    }
    check("hello answers", true)
    check("the palette carries eight topic colours", hello.palette.topic.count == 8,
          "got \(hello.palette.topic.count)")
    check("the palette carries the scheme",
          hello.palette.scheme["accent"] != nil && hello.palette.scheme["danger"] != nil)
    Theme.shared.adopt(hello.palette)
    check("the theme is ready once hello has landed", Theme.shared.ready)

    bridge.send("preview", draft: draft) { reply in
        guard let preview = reply.preview else {
            check("the draft previews", false, reply.error ?? "no reply")
            finish()
        }
        check("the draft previews", true)
        check("two tasks, one note, one skipped",
              preview.tasks == 2 && preview.notes == 1 && preview.skipped == 1,
              "\(preview.tasks)/\(preview.notes)/\(preview.skipped)")

        // The spans must land on the characters that are actually there. This
        // is the one place the bar does index arithmetic of its own.
        let text = draft as NSString
        let spans = Render.spans(for: draft, lines: preview.lines)
        check("every span falls inside the draft",
              spans.allSatisfy { NSMaxRange($0.0) <= text.length })
        check("the separators are coloured as separators",
              spans.filter { $0.1 == "pipe" }.allSatisfy { text.substring(with: $0.0) == "|" },
              spans.filter { $0.1 == "pipe" }.map { text.substring(with: $0.0) }.description)
        if let due = spans.first(where: { $0.1 == "due" }) {
            check("the date is coloured as a date", text.substring(with: due.0) == "today",
                  text.substring(with: due.0))
        } else {
            check("the date is coloured as a date", false, "no due span")
        }
        if let who = spans.first(where: { $0.1 == "who" }) {
            check("the assignee is coloured as one", text.substring(with: who.0) == "@sam",
                  text.substring(with: who.0))
        } else {
            check("the assignee is coloured as one", false, "no who span")
        }

        // The preview says the same thing the terminal says.
        let hues = reply.hues ?? [:]
        let (rendered, marked) = Render.preview(preview, hues: hues, caret: 3)
        let shown = rendered.string
        check("the preview names what each line becomes",
              shown.contains("chase the vendor about the patch")
                  && shown.contains("write the postmortem"))
        check("a note says where it went", shown.contains("attached"))
        check("a line with no separator says why it was left",
              shown.contains("no topic separator"))
        check("the summary counts what the terminal counts",
              Render.summary(preview) == "2 TASKS · 1 NOTE · 1 SKIPPED",
              Render.summary(preview))

        // The line the caret is on is marked, and only that one.
        check("the caret's line is marked", marked != nil)
        if let marked {
            let row = (shown as NSString).substring(with: marked)
            check("the mark is on the line the caret is on",
                  row.contains("write the postmortem"), row.trimmingCharacters(in: .newlines))
        }
        check("only one line is marked",
              shown.components(separatedBy: "▌").count - 1 == 1)

        // The ditto line takes the topic above it, and shares its dot.
        let topics = preview.lines.compactMap { $0.task?.topic }
        check("the ditto line keeps the topic above it",
              topics == ["prod issue", "prod issue"], topics.description)
        check("a topic gets exactly one colour", hues.count == 1, hues.description)

        // ── the keys ────────────────────────────────────────────────────
        //
        // Closing is filing, so the mapping that decides it is worth stating
        // back: esc files, ⌘⌫ throws away, and neither can quietly become the
        // other.
        func press(_ code: UInt16, command: Bool = false,
                   inTitle: Bool = false, canTakeBack: Bool = true) -> Press {
            Keys.press(code: code, command: command, inTitle: inTitle, canTakeBack: canTakeBack)
        }
        check("esc files, because it is the way out", press(Keys.escape) == .file)
        check("⌘↵ files too", press(Keys.enter, command: true) == .file)
        check("⌘⌫ throws the draft away", press(Keys.backspace, command: true) == .scrap)
        check("a bare ⌫ does not", press(Keys.backspace) == .pass)
        check("esc leaves the name of the call before the window",
              press(Keys.escape, inTitle: true) == .leaveTitle)
        check("⌘Z takes back what the window last did",
              press(Keys.z, command: true) == .reverse)
        check("⌘Z is the text view's own with nothing to take back",
              press(Keys.z, command: true, canTakeBack: false) == .pass)
        check("a plain ↵ is a new line, not a filing", press(Keys.enter) == .pass)

        bridge.stop()
        finish()
    }
}

// Nothing here needs a window, but the bridge answers on the main queue.
RunLoop.main.run(until: Date().addingTimeInterval(10))
print("the bridge did not answer within ten seconds")
exit(1)
