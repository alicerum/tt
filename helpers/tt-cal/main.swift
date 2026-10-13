// tt-cal reads macOS Calendar (EventKit) for tt and prints raw events as
// JSON. It has no business rules: tt decides what counts as busy (D17a).
// Build it with build.sh (the calendar usage description must be embedded).
import EventKit
import Foundation

let usage = """
    usage:
      tt-cal calendars
          print the calendar names, one per line (copy them into [calendar] calendars)
      tt-cal events --from <iso8601> --to <iso8601> [--calendars <name>…]
          print the events overlapping from–to as JSON, from the named calendars (default: all)
    exit codes: 0 ok, 1 failure, 2 no calendar access, 3 unknown calendar (its name on stderr)
    """

func fail(_ message: String, _ code: Int32) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(code)
}

// requestAccess asks once (the first run shows the macOS prompt) and exits 2
// when access is denied or restricted.
func requestAccess(_ store: EKEventStore) {
    switch EKEventStore.authorizationStatus(for: .event) {
    case .fullAccess:
        return
    case .notDetermined:
        let answered = DispatchSemaphore(value: 0)
        nonisolated(unsafe) var granted = false
        store.requestFullAccessToEvents { ok, _ in
            granted = ok
            answered.signal()
        }
        answered.wait()
        if !granted {
            fail("no calendar access", 2)
        }
    default:
        fail("no calendar access", 2)
    }
}

let iso: ISO8601DateFormatter = {
    let f = ISO8601DateFormatter()
    f.timeZone = TimeZone.current
    return f
}()

func parseDate(_ s: String) -> Date {
    guard let d = iso.date(from: s) else {
        fail("bad date \"\(s)\": want ISO 8601, like 2026-10-12T04:00:00+02:00", 1)
    }
    return d
}

struct Event: Encodable {
    let title: String
    let start: String
    let end: String
    let allDay: Bool
    let availability: String  // busy, free, tentative, unavailable, or "" when the calendar has none
    let status: String  // my participation: accepted, declined, tentative, pending, or "" when not invited
    let calendar: String

    enum CodingKeys: String, CodingKey {
        case title, start, end, availability, status, calendar
        case allDay = "all_day"
    }
}

func availability(_ a: EKEventAvailability) -> String {
    switch a {
    case .busy: return "busy"
    case .free: return "free"
    case .tentative: return "tentative"
    case .unavailable: return "unavailable"
    default: return ""
    }
}

func status(_ e: EKEvent) -> String {
    guard let me = e.attendees?.first(where: { $0.isCurrentUser }) else {
        return ""
    }
    switch me.participantStatus {
    case .accepted: return "accepted"
    case .declined: return "declined"
    case .tentative: return "tentative"
    case .pending: return "pending"
    default: return ""
    }
}

func listCalendars(_ store: EKEventStore) {
    let names = Set(store.calendars(for: .event).map { $0.title })
    for name in names.sorted() {
        print(name)
    }
}

struct Query {
    let from, to: Date
    let names: [String]?  // nil: all calendars
}

func parseQuery(_ args: [String]) -> Query {
    var from: Date?
    var to: Date?
    var names: [String]?
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--from" where i + 1 < args.count:
            from = parseDate(args[i + 1])
            i += 2
        case "--to" where i + 1 < args.count:
            to = parseDate(args[i + 1])
            i += 2
        case "--calendars":
            names = Array(args[(i + 1)...])
            i = args.count
        default:
            fail(usage, 1)
        }
    }
    guard let from = from, let to = to else {
        fail(usage, 1)
    }
    return Query(from: from, to: to, names: names)
}

func listEvents(_ store: EKEventStore, _ q: Query) {
    var calendars: [EKCalendar]?
    if let names = q.names {
        let all = store.calendars(for: .event)
        calendars = []
        for name in names {
            let matching = all.filter { $0.title == name }
            if matching.isEmpty {
                fail(name, 3)
            }
            calendars! += matching
        }
    }
    let predicate = store.predicateForEvents(withStart: q.from, end: q.to, calendars: calendars)
    let events = store.events(matching: predicate).sorted { $0.startDate < $1.startDate }.map {
        Event(
            title: $0.title ?? "", start: iso.string(from: $0.startDate), end: iso.string(from: $0.endDate),
            allDay: $0.isAllDay, availability: availability($0.availability), status: status($0),
            calendar: $0.calendar.title)
    }
    do {
        FileHandle.standardOutput.write(try JSONEncoder().encode(events))
        print()
    } catch {
        fail("can't encode the events: \(error)", 1)
    }
}

let args = Array(CommandLine.arguments.dropFirst())
guard let command = args.first, command == "calendars" || command == "events" else {
    fail(usage, 1)
}
let query = command == "events" ? parseQuery(Array(args.dropFirst())) : nil  // usage errors before any access
let store = EKEventStore()
requestAccess(store)
if let query = query {
    listEvents(store, query)
} else {
    listCalendars(store)
}
