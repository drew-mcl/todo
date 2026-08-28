// What a keystroke costs in the capture window.
//
// The two things it does on every one of them: paint the grammar over the draft,
// and rebuild the preview. Both are proportional to the whole draft rather than
// the part on screen, which is exactly the shape that made the terminal slow.
//
//	make bench-bar

import AppKit

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

guard let bridge = Bridge() else {
    print("no todo binary; build it first with `make`")
    exit(1)
}
bridge.start()

func pump(_ seconds: Double) {
    RunLoop.main.run(until: Date().addingTimeInterval(seconds))
}

func draft(_ lines: Int) -> String {
    (0..<lines)
        .map { "topic \($0 % 12) | something that needs doing about it | today @sam !! #tag" }
        .joined(separator: "\n")
}

func time(_ what: String, runs: Int = 40, _ body: () -> Void) {
    body() // warm
    let start = Date()
    for _ in 0..<runs { body() }
    let each = Date().timeIntervalSince(start) / Double(runs) * 1000
    print(String(format: "  %-34s %7.3f ms", (what as NSString).utf8String!, each))
}

var greeted: Hello?
bridge.send("hello") { greeted = $0.hello }
let deadline = Date().addingTimeInterval(10)
while greeted == nil && Date() < deadline { pump(0.05) }
guard let hello = greeted else {
    print("todo bridge did not answer")
    exit(1)
}
Theme.shared.adopt(hello.palette)
Theme.shared.adopt(hello.keys ?? [])

for lines in [20, 200, 1000] {
    let text = draft(lines)
    var parsed: Preview?
    var hues: [String: Int] = [:]
    bridge.send("preview", draft: text) { reply in
        parsed = reply.preview
        hues = reply.hues ?? [:]
    }
    let until = Date().addingTimeInterval(10)
    while parsed == nil && Date() < until { pump(0.02) }
    guard let preview = parsed else {
        print("the parse did not come back for \(lines) lines")
        exit(1)
    }

    print("\(lines) lines")
    time("Render.preview") { _ = Render.preview(preview, hues: hues, caret: lines) }
    time("Render.spans") { _ = Render.spans(for: text, lines: preview.lines) }
    time("Vim.press w") {
        _ = Vim.press("w", VimState(text: text, at: text.utf16.count / 2, mode: .normal))
    }
}

// Where a block's cost actually is, since the page is linear in blocks and
// twenty-seven microseconds is a lot for six short runs of text.
print("one block, 200 times")
let ink = Theme.shared.colour("ink")
time("append + setAttributes x6") {
    let page = Page()
    page.begin()
    for _ in 0..<200 {
        for _ in 0..<6 { page.put("something that needs doing", Type.mono, ink) }
    }
    page.end()
}
time("append only x6") {
    let out = NSMutableAttributedString()
    out.beginEditing()
    for _ in 0..<200 {
        for _ in 0..<6 { out.mutableString.append("something that needs doing") }
    }
    out.endEditing()
}
let plain = NSAttributedString(string: "something that needs doing",
                               attributes: [.font: Type.mono, .foregroundColor: ink])
time("append prebuilt attributed x6") {
    let out = NSMutableAttributedString()
    out.beginEditing()
    for _ in 0..<200 {
        for _ in 0..<6 { out.append(plain) }
    }
    out.endEditing()
}

bridge.stop()
exit(0)
