import AppKit

/// The window itself.
///
/// It joins every Space and sits over full-screen apps, because the whole point
/// is that the thing you were doing when the commitment was made is still on
/// screen behind it. It does not take a Space of its own and it does not send
/// you back to a desktop to reach it.
final class CapturePanel: NSPanel {
    init(size: NSSize = NSSize(width: 720, height: 470)) {
        super.init(
            contentRect: NSRect(origin: .zero, size: size),
            styleMask: [.titled, .fullSizeContentView, .nonactivatingPanel, .resizable],
            backing: .buffered, defer: false)

        isFloatingPanel = true
        level = .floating
        collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .ignoresCycle]
        titlebarAppearsTransparent = true
        titleVisibility = .hidden
        isMovableByWindowBackground = true
        hidesOnDeactivate = false
        animationBehavior = .utilityWindow
        isReleasedWhenClosed = false
        minSize = NSSize(width: 520, height: 320)
        for button in [NSWindow.ButtonType.closeButton, .miniaturizeButton, .zoomButton] {
            standardWindowButton(button)?.isHidden = true
        }

        let backdrop = NSVisualEffectView()
        backdrop.material = .popover
        backdrop.blendingMode = .behindWindow
        backdrop.state = .active
        contentView = backdrop
    }

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }

    /// Put the window where the eyes already are: the screen holding the mouse,
    /// a little above centre, which is where every other summoned window on this
    /// machine appears.
    func positionOnActiveScreen() {
        let mouse = NSEvent.mouseLocation
        let screen = NSScreen.screens.first { $0.frame.contains(mouse) }
            ?? NSScreen.main
        guard let visible = screen?.visibleFrame else { return }
        let size = frame.size
        setFrameOrigin(NSPoint(
            x: visible.midX - size.width / 2,
            y: visible.minY + (visible.height - size.height) * 0.68))
    }
}

/// Who we interrupted.
///
/// Both windows put the keyboard back where they found it when they close. Held
/// here rather than in each of them, because hopping from the box to the lists
/// used to hand focus to the app underneath and immediately take it again --
/// one flicker per keystroke, and whichever app happened to be frontmost during
/// it became the one we thought we had interrupted.
enum Interrupted {
    private(set) static var app: NSRunningApplication?

    /// Remembered once, on the way in, and not overwritten by our own windows.
    static func remember() {
        guard app == nil else { return }
        let front = NSWorkspace.shared.frontmostApplication
        guard front?.processIdentifier != ProcessInfo.processInfo.processIdentifier else { return }
        app = front
    }

    static func restore() {
        app?.activate()
        app = nil
    }
}
