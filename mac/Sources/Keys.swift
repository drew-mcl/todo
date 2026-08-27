import Foundation

// What a key means in this window.
//
// Closing is filing. A capture box you have finished with is a capture box
// whose contents you meant, so the way out does the useful thing and throwing
// the draft away is the keystroke you have to mean. That is a decision worth
// stating in one table rather than leaving spread through an event handler --
// and a table can be read back without a screen, which a window cannot.

enum Press: Equatable {
    /// File what is there and close. What esc does, because it is the way out.
    case file
    /// Throw the draft away and close, recoverably.
    case scrap
    /// Put back whatever the last one of those did, words and all.
    case reverse
    /// Out of the name of the call before out of the window.
    case leaveTitle
    /// Not ours; let the text view have it.
    case pass
}

enum Keys {
    static let escape: UInt16 = 53
    static let enter: UInt16 = 36
    static let backspace: UInt16 = 51
    static let z: UInt16 = 6

    static func press(code: UInt16, command: Bool, inTitle: Bool, canTakeBack: Bool) -> Press {
        switch (code, command) {
        case (escape, _):
            return inTitle ? .leaveTitle : .file
        // ⌘↵ files too, for hands that learned it here or in the browser.
        case (enter, true):
            return .file
        case (backspace, true):
            return .scrap
        case (z, true):
            return canTakeBack ? .reverse : .pass
        default:
            return .pass
        }
    }
}
