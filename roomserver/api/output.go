// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// An OutputType is a type of roomserver output.
type OutputType string

const (
	// OutputTypeNewRoomEvent indicates that the event is an OutputNewRoomEvent
	OutputTypeNewRoomEvent OutputType = "new_room_event"
	// OutputTypeNewInviteEvent indicates that the event is an OutputNewInviteEvent
	OutputTypeNewInviteEvent OutputType = "new_invite_event"
	// OutputTypeRetireInviteEvent indicates that the event is an OutputRetireInviteEvent
	OutputTypeRetireInviteEvent OutputType = "retire_invite_event"
	// OutputTypeRedactedEvent indicates that the event is an OutputRedactedEvent
	//
	// This event is emitted when a redaction has been 'validated' (meaning both the redaction and the event to redact are known).
	// Redaction validation happens when the roomserver receives either:
	// - A redaction for which we have the event to redact.
	// - Any event for which we have a redaction.
	// When the roomserver receives an event, it will check against the redactions table to see if there is a matching redaction
	// for the event. If there is, it will mark the redaction as validated and emit this event. In the common case of a redaction
	// happening after receiving the event to redact, the roomserver will emit a OutputTypeNewRoomEvent of m.room.redaction
	// immediately followed by a OutputTypeRedactedEvent. In the uncommon case of receiving the redaction BEFORE the event to redact,
	// the roomserver will emit a OutputTypeNewRoomEvent of the event to redact immediately followed by a OutputTypeRedactedEvent.
	//
	// In order to honour redactions correctly, downstream components must ignore m.room.redaction events emitted via OutputTypeNewRoomEvent.
	// When downstream components receive an OutputTypeRedactedEvent they must:
	// - Pull out the event to redact from the database. They should have this because the redaction is validated.
	// - Redact the event and set the corresponding `unsigned` fields to indicate it as redacted.
	// - Replace the event in the database.
	OutputTypeRedactedEvent OutputType = "redacted_event"
)

// An OutputEvent is an entry in the roomserver output kafka log.
// Consumers should check the type field when consuming this event.
type OutputEvent struct {
	// What sort of event this is.
	Type OutputType `json:"type"`
	// The content of event with type OutputTypeNewRoomEvent
	NewRoomEvent *OutputNewRoomEvent `json:"new_room_event,omitempty"`
	// The content of event with type OutputTypeNewInviteEvent
	NewInviteEvent *OutputNewInviteEvent `json:"new_invite_event,omitempty"`
	// The content of event with type OutputTypeRetireInviteEvent
	RetireInviteEvent *OutputRetireInviteEvent `json:"retire_invite_event,omitempty"`
	// The content of event with type  OutputTypeRedactedEvent
	RedactedEvent *OutputRedactedEvent `json:"redacted_event,omitempty"`
}

// An OutputNewRoomEvent is written when the roomserver receives a new event.
// It contains the full matrix room event and enough information for a
// consumer to construct the current state of the room and the state before the
// event.
//
// When we talk about state in a matrix room we are talking about the state
// after a list of events. The current state is the state after the latest
// event IDs in the room. The state before an event is the state after its
// prev_events.
type OutputNewRoomEvent struct {
	// The Event.
	Event gomatrixserverlib.HeaderedEvent `json:"event"`
	// Is the event historical? If so, then downstream components should not treat the
	// event as if it just arrived.
	Historical bool `json:"historical"`
	// The latest events in the room after this event.
	// This can be used to set the prev events for new events in the room.
	// This also can be used to get the full current state after this event.
	LatestEventIDs []string `json:"latest_event_ids"`
	// The state event IDs that were added to the state of the room by this event.
	// Together with RemovesStateEventIDs this allows the receiver to keep an up to date
	// view of the current state of the room.
	AddsStateEventIDs []string `json:"adds_state_event_ids"`
	// All extra newly added state events. This is only set if there are *extra* events
	// other than `Event`. This can happen when forks get merged because state resolution
	// may decide a bunch of state events on one branch are now valid, so they will be
	// present in this list. This is useful when trying to maintain the current state of a room
	// as to do so you need to include both these events and `Event`.
	AddStateEvents []gomatrixserverlib.HeaderedEvent `json:"adds_state_events"`

	// The state event IDs that were removed from the state of the room by this event.
	RemovesStateEventIDs []string `json:"removes_state_event_ids"`
	// The ID of the event that was output before this event.
	// Or the empty string if this is the first event output for this room.
	// This is used by consumers to check if they can safely update their
	// current state using the delta supplied in AddsStateEventIDs and
	// RemovesStateEventIDs.
	//
	// If the LastSentEventID doesn't match what they were expecting it to be
	// they can use the LatestEventIDs to request the full current state.
	LastSentEventID string `json:"last_sent_event_id"`
	// The state event IDs that are part of the state at the event, but not
	// part of the current state. Together with the StateBeforeRemovesEventIDs
	// this can be used to construct the state before the event from the
	// current state. The StateBeforeAddsEventIDs and StateBeforeRemovesEventIDs
	// delta is applied after the AddsStateEventIDs and RemovesStateEventIDs.
	//
	// Consumers need to know the state at each event in order to determine
	// which users and servers are allowed to see the event. This information
	// is needed to apply the history visibility rules and to tell which
	// servers we need to push events to over federation.
	//
	// The state is given as a delta against the current state because they are
	// usually either the same state, or differ by just a couple of events.
	StateBeforeAddsEventIDs []string `json:"state_before_adds_event_ids"`
	// The state event IDs that are part of the current state, but not part
	// of the state at the event.
	StateBeforeRemovesEventIDs []string `json:"state_before_removes_event_ids"`
	// The server name to use to push this event to other servers.
	// Or empty if this event shouldn't be pushed to other servers.
	//
	// This is used by the federation sender component. We need to tell it what
	// event it needs to send because it can't tell on its own. Normally if an
	// event was created on this server then we are responsible for sending it.
	// However there are a couple of exceptions. The first is that when the
	// server joins a remote room through another matrix server, it is the job
	// of the other matrix server to send the event over federation. The second
	// is the reverse of the first, that is when a remote server joins a room
	// that we are in over federation using our server it is our responsibility
	// to send the join event to other matrix servers.
	//
	// We encode the server name that the event should be sent using here to
	// future proof the API for virtual hosting.
	SendAsServer string `json:"send_as_server"`
	// The transaction ID of the send request if sent by a local user and one
	// was specified
	TransactionID *TransactionID `json:"transaction_id"`
}

// AddsState returns all added state events from this event.
//
// This function is needed because `AddStateEvents` will not include a copy of
// the original event to save space, so you cannot use that slice alone.
// Instead, use this function which will add the original event if it is present
// in `AddsStateEventIDs`.
func (ore *OutputNewRoomEvent) AddsState() []gomatrixserverlib.HeaderedEvent {
	includeOutputEvent := false
	for _, id := range ore.AddsStateEventIDs {
		if id == ore.Event.EventID() {
			includeOutputEvent = true
			break
		}
	}
	if !includeOutputEvent {
		return ore.AddStateEvents
	}
	return append(ore.AddStateEvents, ore.Event)
}

// An OutputNewInviteEvent is written whenever an invite becomes active.
// Invite events can be received outside of an existing room so have to be
// tracked separately from the room events themselves.
type OutputNewInviteEvent struct {
	// The room version of the invited room.
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
	// The "m.room.member" invite event.
	Event gomatrixserverlib.HeaderedEvent `json:"event"`
}

// An OutputRetireInviteEvent is written whenever an existing invite is no longer
// active. An invite stops being active if the user joins the room or if the
// invite is rejected by the user.
type OutputRetireInviteEvent struct {
	// The ID of the "m.room.member" invite event.
	EventID string
	// The target user ID of the "m.room.member" invite event that was retired.
	TargetUserID string
	// Optional event ID of the event that replaced the invite.
	// This can be empty if the invite was rejected locally and we were unable
	// to reach the server that originally sent the invite.
	RetiredByEventID string
	// The "membership" of the user after retiring the invite. One of "join"
	// "leave" or "ban".
	Membership string
}

// An OutputRedactedEvent is written whenever a redaction has been /validated/.
// Downstream components MUST redact the given event ID if they have stored the
// event JSON. It is guaranteed that this event ID has been seen before.
type OutputRedactedEvent struct {
	// The event ID that was redacted
	RedactedEventID string
	// The value of `unsigned.redacted_because` - the redaction event itself
	RedactedBecause gomatrixserverlib.HeaderedEvent
}
