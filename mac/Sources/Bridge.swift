import Foundation

// The bar knows the grammar the way the browser does: not at all.
//
// `todo bridge` is one long-lived child process speaking a line of JSON in and
// a line of JSON out. Every date, every colour and every decision about what a
// line means is made on the other side of that pipe, so this window cannot
// drift from the terminal app or the web one -- there is nothing here to drift.

struct Colour: Decodable {
    let light: String
    let dark: String
}

struct Palette: Decodable {
    let topic: [Colour]
    let scheme: [String: Colour]
}

struct KeyGroup: Decodable {
    struct Key: Decodable {
        let press: String
        let does: String
    }
    let name: String
    let keys: [Key]
}

struct Hello: Decodable {
    let version: String
    let palette: Palette
    let counts: [String: Int]
    let keys: [KeyGroup]?
}

struct Token: Decodable {
    let kind: String
    let text: String
}

struct PreviewTask: Decodable {
    let topic: String
    let title: String
    let note: String
    let dueLabel: String
    let assignee: String
    let priority: Int
    let tags: [String]
    let warning: String
}

struct PreviewLine: Decodable {
    let n: Int
    let raw: String
    let kind: String
    let reason: String?
    let tokens: [Token]?
    let task: PreviewTask?
    let ownerTitle: String?
}

struct Preview: Decodable {
    let lines: [PreviewLine]
    let tasks: Int
    let notes: Int
    let skipped: Int
}

struct Task: Decodable {
    let id: Int
    let topic: String
    let title: String
    let dueLabel: String
    let assignee: String
    let priority: Int
}

struct Section: Decodable {
    let label: String
    let late: Bool
    let tasks: [Task]
}

struct Day: Decodable {
    let view: String
    let label: String
    let sections: [Section]
    let done: Int
    let open: Int
    let total: Int?
    let truncated: Bool?
    let hues: [String: Int]
}

struct Added: Decodable {
    let batchId: Int
    let added: Int
    let today: Int
}

struct Reply: Decodable {
    let id: Int
    let error: String?
    let hello: Hello?
    let preview: Preview?
    let hues: [String: Int]?
    let added: Added?
    let undone: Int?
    let day: Day?
}

/// Bridge owns the child process and matches replies back to the calls that
/// asked for them.
final class Bridge {
    /// Raised when the child cannot be started or has stopped answering.
    var onBroken: ((String) -> Void)?

    private var process: Process?
    private var stdin: FileHandle?
    private var pending: [Int: (Reply) -> Void] = [:]
    private var nextID = 0
    private var buffer = Data()
    private var stopping = false
    private let binary: String

    init?() {
        guard let found = Bridge.locate() else { return nil }
        binary = found
    }

    /// The path to `todo`. Told, then remembered, then looked for in the
    /// obvious places, then asked of a login shell -- which is the only one of
    /// those that knows what a user's PATH actually is.
    static func locate() -> String? {
        let fm = FileManager.default
        var candidates: [String] = []

        if let env = ProcessInfo.processInfo.environment["TODO_BIN"] {
            candidates.append(env)
        }
        if let saved = UserDefaults.standard.string(forKey: "binary") {
            candidates.append(saved)
        }
        // Inside the bundle, where `make bar` puts it. A GUI application is
        // handed a bare environment -- no shell, no PATH, and a login shell
        // will not read the file that sets one -- so the only reliable answer
        // is the one that travels with the app.
        if let beside = Bundle.main.executableURL?.deletingLastPathComponent() {
            candidates.append(beside.appendingPathComponent("todo").path)
        }
        // And next to it, for a bar that was built but not installed.
        let bundle = Bundle.main.bundleURL.deletingLastPathComponent()
        candidates.append(bundle.appendingPathComponent("todo").path)
        candidates += [
            "/opt/homebrew/bin/todo",
            "/usr/local/bin/todo",
            NSHomeDirectory() + "/go/bin/todo",
        ]
        for path in candidates where fm.isExecutableFile(atPath: path) {
            return path
        }

        let shell = ProcessInfo.processInfo.environment["SHELL"] ?? "/bin/zsh"
        let ask = Process()
        ask.executableURL = URL(fileURLWithPath: shell)
        ask.arguments = ["-lc", "command -v todo"]
        let out = Pipe()
        ask.standardOutput = out
        ask.standardError = FileHandle.nullDevice
        do {
            try ask.run()
            ask.waitUntilExit()
        } catch {
            return nil
        }
        let found = String(decoding: out.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return fm.isExecutableFile(atPath: found) ? found : nil
    }

    func start() {
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: binary)
        proc.arguments = ["bridge"]

        let input = Pipe(), output = Pipe()
        proc.standardInput = input
        proc.standardOutput = output
        proc.standardError = FileHandle.nullDevice

        output.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let chunk = handle.availableData
            guard !chunk.isEmpty else { return }
            DispatchQueue.main.async { self?.absorb(chunk) }
        }
        proc.terminationHandler = { [weak self] _ in
            DispatchQueue.main.async {
                guard let self, !self.stopping else { return }
                self.process = nil
                self.onBroken?("todo bridge stopped")
            }
        }

        do {
            try proc.run()
        } catch {
            onBroken?("could not start \(binary): \(error.localizedDescription)")
            return
        }
        process = proc
        stdin = input.fileHandleForWriting
    }

    /// A reply arrives whole or not at all, so the buffer is cut on newlines
    /// and anything after the last one waits for the rest of itself.
    private func absorb(_ chunk: Data) {
        buffer.append(chunk)
        while let end = buffer.firstIndex(of: 0x0A) {
            let line = buffer[buffer.startIndex..<end]
            buffer.removeSubrange(buffer.startIndex...end)
            guard let reply = try? JSONDecoder().decode(Reply.self, from: line) else { continue }
            pending.removeValue(forKey: reply.id)?(reply)
        }
    }

    @discardableResult
    func send(_ op: String, draft: String = "", title: String = "", batch: Int = 0,
              task: Int = 0, view: String = "",
              then handler: @escaping (Reply) -> Void) -> Int {
        if process == nil && !stopping { start() }
        nextID += 1
        let id = nextID
        pending[id] = handler

        var body: [String: Any] = ["id": id, "op": op]
        if !draft.isEmpty { body["draft"] = draft }
        if !title.isEmpty { body["title"] = title }
        if batch != 0 { body["batch"] = batch }
        if task != 0 { body["task"] = task }
        if !view.isEmpty { body["view"] = view }

        guard var data = try? JSONSerialization.data(withJSONObject: body) else { return id }
        data.append(0x0A)
        do {
            try stdin?.write(contentsOf: data)
        } catch {
            pending.removeValue(forKey: id)
            onBroken?("todo bridge stopped listening")
        }
        return id
    }

    func stop() {
        stopping = true
        stdin?.closeFile()
        process?.terminate()
        process = nil
    }
}
