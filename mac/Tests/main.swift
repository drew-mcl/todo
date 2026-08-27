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
        // A note is shown under the task it belongs to, not as a line of its own:
        // printing it twice made the list look like it had read the line twice.
        check("a note is shown under its task",
              shown.contains("↳ they have missed two dates now"))
        check("a note does not also get a block of its own",
              shown.components(separatedBy: "they have missed").count - 1 == 1)
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
        check("only the caret's block is lit",
              marked.map { $0.length > 0 && NSMaxRange($0) <= (shown as NSString).length } ?? false)

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

        // ── normal mode ─────────────────────────────────────────────────
        //
        // The sheet the window shows comes from the same list this side is
        // meant to implement, so every key on it is pressed here. A reference
        // that promises a key nobody wrote is worse than no reference.
        func after(_ keys: [String], on text: String = "one two\nthree four",
                   at: Int = 4) -> VimResult {
            var state = VimState(text: text, at: at, mode: .normal)
            var out = VimResult(state: state, handled: false)
            for key in keys {
                out = Vim.press(key, state)
                state = out.state
            }
            return out
        }

        check("esc leaves typing without filing", {
            let out = Vim.press("\u{1b}", VimState(text: "hi", at: 2, mode: .insert))
            return out.handled && out.state.mode == .normal && out.exit == nil
        }())
        check("esc from normal files", after(["\u{1b}"]).exit == .file)
        check("so does return, once you have stopped typing",
              after(["\r"]).exit == .file
                  && !Vim.press("\r", VimState(text: "a", at: 1, mode: .insert)).handled)
        check("ZZ files, ZQ scraps",
              after(["Z", "Z"]).exit == .file && after(["Z", "Q"]).exit == .scrap)
        check("dd takes the line and its newline",
              after(["d", "d"]).state.text == "three four",
              after(["d", "d"]).state.text)
        check("D takes the rest of the line",
              after(["D"]).state.text == "one \nthree four", after(["D"]).state.text)
        check("cc clears the line and types",
              after(["c", "c"]).state.text == "\nthree four"
                  && after(["c", "c"]).state.mode == .insert)
        check("x takes one character", after(["x"]).state.text == "one wo\nthree four",
              after(["x"]).state.text)
        // "one two\nthree four", caret on the t of two: w reaches three.
        check("w goes on to the next word", after(["w"]).state.at == 8,
              "\(after(["w"]).state.at)")
        check("b goes back a word", after(["b"]).state.at == 0, "\(after(["b"]).state.at)")
        check("e reaches the end of a word", after(["e"]).state.at == 6,
              "\(after(["e"]).state.at)")
        check("gg and G reach the ends",
              after(["g", "g"]).state.at == 0 && after(["G"]).state.at == 18,
              "\(after(["G"]).state.at)")
        check("0 and $ reach the ends of the line",
              after(["0"]).state.at == 0 && after(["$"]).state.at == 7)
        check("j and k hold the column",
              after(["j"]).state.at == 12 && after(["j", "k"]).state.at == 4,
              "\(after(["j"]).state.at)")
        check("i a I A o O all start typing",
              ["i", "a", "I", "A", "o", "O"].allSatisfy { after([$0]).state.mode == .insert })
        check("a stray letter never reaches the draft", {
            let out = after(["q"])
            return out.handled && out.state.text == "one two\nthree four"
        }())
        check("every key while typing belongs to the text view",
              ["d", "w", "x", "G", "?"].allSatisfy {
                  !Vim.press($0, VimState(text: "", at: 0, mode: .insert)).handled
              })

        // And the sheet the window would draw is the one the bridge sent.
        let sheet = Render.sheet(hello.keys ?? []).string
        check("the reference names the keys it was sent",
              !(hello.keys ?? []).isEmpty && sheet.contains("dd") && sheet.contains("gg"),
              sheet.prefix(60).description)

        bridge.stop()
        finish()
    }
}

// Nothing here needs a window, but the bridge answers on the main queue.
RunLoop.main.run(until: Date().addingTimeInterval(10))
print("the bridge did not answer within ten seconds")
exit(1)
