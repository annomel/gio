// SPDX-License-Identifier: Unlicense OR MIT

package pointer

import (
	"encoding/binary"
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/internal/ops"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/op"
)

// Event is a pointer event.
type Event struct {
	Type   Type
	Source Source
	// PointerID is the id for the pointer and can be used
	// to track a particular pointer from Press to
	// Release or Cancel.
	PointerID ID
	// Priority is the priority of the receiving handler
	// for this event.
	Priority Priority
	// Time is when the event was received. The
	// timestamp is relative to an undefined base.
	Time time.Duration
	// Buttons are the set of pressed mouse buttons for this event.
	Buttons Buttons
	// Position is the position of the event, relative to
	// the current transformation, as set by op.TransformOp.
	Position f32.Point
	// Scroll is the scroll amount, if any.
	Scroll f32.Point
	// Modifiers is the set of active modifiers when
	// the mouse button was pressed.
	Modifiers key.Modifiers
}

// AreaOp pushes the current hit area to the stack and updates it to the
// intersection of the current hit area and the transformed area.
type AreaOp struct {
	kind areaKind
	rect image.Rectangle
}

// AreaStack represents an AreaOp on the stack of areas.
type AreaStack struct {
	ops     *ops.Ops
	id      ops.StackID
	macroID int
}

// PassOp sets the pass-through mode. AreaOps added while the pass-through
// mode is set don't block events to siblings.
type PassOp struct {
}

// PassStack represents a PassOp on the pass stack.
type PassStack struct {
	ops     *ops.Ops
	id      ops.StackID
	macroID int
}

// CursorNameOp sets the cursor for the current area.
type CursorNameOp struct {
	Name CursorName
}

// InputOp declares an input handler ready for pointer
// events.
type InputOp struct {
	Tag event.Tag
	// Grab, if set, request that the handler get
	// Grabbed priority.
	Grab bool
	// Types is a bitwise-or of event types to receive.
	Types Type
	// ScrollBounds describe the maximum scrollable distances in both
	// axes. Specifically, any Event e delivered to Tag will satisfy
	//
	// ScrollBounds.Min.X <= e.Scroll.X <= ScrollBounds.Max.X (horizontal axis)
	// ScrollBounds.Min.Y <= e.Scroll.Y <= ScrollBounds.Max.Y (vertical axis)
	ScrollBounds image.Rectangle
}

type ID uint16

// Type of an Event.
type Type uint8

// Priority of an Event.
type Priority uint8

// Source of an Event.
type Source uint8

// Buttons is a set of mouse buttons
type Buttons uint8

// CursorName is the name of a cursor.
type CursorName string

// Must match app/internal/input.areaKind
type areaKind uint8

const (
	// CursorDefault is the default cursor.
	CursorDefault CursorName = ""
	// CursorText is the cursor for text.
	CursorText CursorName = "text"
	// CursorPointer is the cursor for a link.
	CursorPointer CursorName = "pointer"
	// CursorCrossHair is the cursor for precise location.
	CursorCrossHair CursorName = "crosshair"
	// CursorColResize is the cursor for vertical resize.
	CursorColResize CursorName = "col-resize"
	// CursorRowResize is the cursor for horizontal resize.
	CursorRowResize CursorName = "row-resize"
	// CursorGrab is the cursor for moving object in any direction.
	CursorGrab CursorName = "grab"
	// CursorNone hides the cursor. To show it again, use any other cursor.
	CursorNone CursorName = "none"
)

const (
	// A Cancel event is generated when the current gesture is
	// interrupted by other handlers or the system.
	Cancel Type = (1 << iota) >> 1
	// Press of a pointer.
	Press
	// Release of a pointer.
	Release
	// Move of a pointer.
	Move
	// Drag of a pointer.
	Drag
	// Pointer enters an area watching for pointer input
	Enter
	// Pointer leaves an area watching for pointer input
	Leave
	// Scroll of a pointer.
	Scroll
)

const (
	// Mouse generated event.
	Mouse Source = iota
	// Touch generated event.
	Touch
)

const (
	// Shared priority is for handlers that
	// are part of a matching set larger than 1.
	Shared Priority = iota
	// Foremost priority is like Shared, but the
	// handler is the foremost of the matching set.
	Foremost
	// Grabbed is used for matching sets of size 1.
	Grabbed
)

const (
	// ButtonPrimary is the primary button, usually the left button for a
	// right-handed user.
	ButtonPrimary Buttons = 1 << iota
	// ButtonSecondary is the secondary button, usually the right button for a
	// right-handed user.
	ButtonSecondary
	// ButtonTertiary is the tertiary button, usually the middle button.
	ButtonTertiary
)

const (
	areaRect areaKind = iota
	areaEllipse
)

// Rect constructs a rectangular hit area.
func Rect(size image.Rectangle) AreaOp {
	return AreaOp{
		kind: areaRect,
		rect: size,
	}
}

// Ellipse constructs an ellipsoid hit area.
func Ellipse(size image.Rectangle) AreaOp {
	return AreaOp{
		kind: areaEllipse,
		rect: size,
	}
}

// Push the current area to the stack and intersects the current area with the
// area represented by o.
func (a AreaOp) Push(o *op.Ops) AreaStack {
	id, macroID := ops.PushOp(&o.Internal, ops.AreaStack)
	a.add(o, true)
	return AreaStack{ops: &o.Internal, id: id, macroID: macroID}
}

func (a AreaOp) add(o *op.Ops, push bool) {
	data := ops.Write(&o.Internal, ops.TypeAreaLen)
	data[0] = byte(ops.TypeArea)
	data[1] = byte(a.kind)
	bo := binary.LittleEndian
	bo.PutUint32(data[2:], uint32(a.rect.Min.X))
	bo.PutUint32(data[6:], uint32(a.rect.Min.Y))
	bo.PutUint32(data[10:], uint32(a.rect.Max.X))
	bo.PutUint32(data[14:], uint32(a.rect.Max.Y))
}

func (o AreaStack) Pop() {
	ops.PopOp(o.ops, ops.AreaStack, o.id, o.macroID)
	data := ops.Write(o.ops, ops.TypePopAreaLen)
	data[0] = byte(ops.TypePopArea)
}

// Push the current pass mode to the pass stack and set the pass mode.
func (p PassOp) Push(o *op.Ops) PassStack {
	id, mid := ops.PushOp(&o.Internal, ops.PassStack)
	data := ops.Write(&o.Internal, ops.TypePassLen)
	data[0] = byte(ops.TypePass)
	return PassStack{ops: &o.Internal, id: id, macroID: mid}
}

func (p PassStack) Pop() {
	ops.PopOp(p.ops, ops.PassStack, p.id, p.macroID)
	data := ops.Write(p.ops, ops.TypePopPassLen)
	data[0] = byte(ops.TypePopPass)
}

func (op CursorNameOp) Add(o *op.Ops) {
	data := ops.Write1(&o.Internal, ops.TypeCursorLen, op.Name)
	data[0] = byte(ops.TypeCursor)
}

// Add panics if the scroll range does not contain zero.
func (op InputOp) Add(o *op.Ops) {
	if op.Tag == nil {
		panic("Tag must be non-nil")
	}
	if b := op.ScrollBounds; b.Min.X > 0 || b.Max.X < 0 || b.Min.Y > 0 || b.Max.Y < 0 {
		panic(fmt.Errorf("invalid scroll range value %v", b))
	}
	data := ops.Write1(&o.Internal, ops.TypePointerInputLen, op.Tag)
	data[0] = byte(ops.TypePointerInput)
	if op.Grab {
		data[1] = 1
	}
	data[2] = byte(op.Types)
	bo := binary.LittleEndian
	bo.PutUint32(data[3:], uint32(op.ScrollBounds.Min.X))
	bo.PutUint32(data[7:], uint32(op.ScrollBounds.Min.Y))
	bo.PutUint32(data[11:], uint32(op.ScrollBounds.Max.X))
	bo.PutUint32(data[15:], uint32(op.ScrollBounds.Max.Y))
}

func (t Type) String() string {
	switch t {
	case Press:
		return "Press"
	case Release:
		return "Release"
	case Cancel:
		return "Cancel"
	case Move:
		return "Move"
	case Drag:
		return "Drag"
	case Enter:
		return "Enter"
	case Leave:
		return "Leave"
	case Scroll:
		return "Scroll"
	default:
		panic("unknown Type")
	}
}

func (p Priority) String() string {
	switch p {
	case Shared:
		return "Shared"
	case Foremost:
		return "Foremost"
	case Grabbed:
		return "Grabbed"
	default:
		panic("unknown priority")
	}
}

func (s Source) String() string {
	switch s {
	case Mouse:
		return "Mouse"
	case Touch:
		return "Touch"
	default:
		panic("unknown source")
	}
}

// Contain reports whether the set b contains
// all of the buttons.
func (b Buttons) Contain(buttons Buttons) bool {
	return b&buttons == buttons
}

func (b Buttons) String() string {
	var strs []string
	if b.Contain(ButtonPrimary) {
		strs = append(strs, "ButtonPrimary")
	}
	if b.Contain(ButtonSecondary) {
		strs = append(strs, "ButtonSecondary")
	}
	if b.Contain(ButtonTertiary) {
		strs = append(strs, "ButtonTertiary")
	}
	return strings.Join(strs, "|")
}

func (c CursorName) String() string {
	if c == CursorDefault {
		return "default"
	}
	return string(c)
}

func (Event) ImplementsEvent() {}
