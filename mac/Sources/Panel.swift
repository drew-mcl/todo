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
