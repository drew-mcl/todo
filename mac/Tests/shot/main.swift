// Draw the capture window to a file, so it can be looked at.
//
// The window is the one part of this that cannot be asserted about -- it either
// reads well or it does not, and that needs eyes. Taking a screenshot needs a
// permission and a person at the keyboard; a view drawing itself needs neither.
//
//	make shot                        the sample draft
//	make shot DRAFT="topic | thing"  your own
//
// Writes mac/build/shot.png.

import AppKit

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

let env = ProcessInfo.processInfo.environment
let draft = env["DRAFT"].flatMap { $0.isEmpty ? nil : $0 } ?? """
TESTING TESTING | this looks weird, the live render is not right
hel
prod issue | chase the vendor about the patch | today @sam !!
> they have missed two dates now
test |
"""
let out = env["SHOT"].flatMap { $0.isEmpty ? nil : $0 } ?? "mac/build/shot.png"

guard let bridge = Bridge() else {
    print("no todo binary; build it first with `make`")
    exit(1)
}
bridge.start()

func pump(_ seconds: Double) {
    RunLoop.main.run(until: Date().addingTimeInterval(seconds))
}

/// The draft view is the one inside the first scroll view.
func draftView(_ root: NSView) -> NSTextView? {
    for view in root.subviews {
        if let scroll = view as? NSScrollView, let text = scroll.documentView as? NSTextView {
            return text
        }
    }
    return nil
}

// Flat, not nested: pumping a run loop from inside a reply handler leaves the
// next reply waiting on the handler that is waiting for it.
var greeted: Hello?
var failed: String?
bridge.send("hello") { reply in
    greeted = reply.hello
    failed = reply.error ?? (reply.hello == nil ? "no hello" : nil)
}
let deadline = Date().addingTimeInterval(10)
while greeted == nil && failed == nil && Date() < deadline { pump(0.05) }

guard let hello = greeted else {
    print("todo bridge did not answer: \(failed ?? "timed out")")
    exit(1)
}
Theme.shared.adopt(hello.palette)
Theme.shared.adopt(hello.keys ?? [])

let capture = CaptureController(bridge: bridge)
capture.panel.setContentSize(NSSize(width: 720, height: 470))
capture.panel.orderFront(nil)

guard let root = capture.panel.contentView, let text = draftView(root) else {
    print("the window did not build")
    exit(1)
}
text.string = draft
text.didChangeText()
if env["MODE"] == "normal" {
    capture.pressForShot("\u{1b}")
}

// The parse is a debounce and a round trip away.
pump(1.2)
root.layoutSubtreeIfNeeded()
root.display()

guard let rep = root.bitmapImageRepForCachingDisplay(in: root.bounds) else {
    print("could not make a bitmap")
    exit(1)
}
root.cacheDisplay(in: root.bounds, to: rep)
guard let png = rep.representation(using: .png, properties: [:]) else {
    print("could not encode a png")
    exit(1)
}
do {
    try png.write(to: URL(fileURLWithPath: out))
    print("wrote \(out)  \(Int(root.bounds.width))x\(Int(root.bounds.height))")
} catch {
    print("could not write \(out): \(error.localizedDescription)")
    exit(1)
}
bridge.stop()
exit(0)
