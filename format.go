/*
 * Copyright (c) 2013-2016 Dave Collins <dave@davec.name>
 * Copyright (c) 2021 Anner van Hardenbroek
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package spew

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// supportedFlags is a list of all the character flags supported by fmt package.
const supportedFlags = "0-+# "

// formatState implements the fmt.Formatter interface and contains information
// about the state of a formatting operation.  The NewFormatter function can
// be used to get a new Formatter which can be used directly as arguments
// in standard fmt package printing calls.
type formatState struct {
	value          interface{}
	fs             fmt.State
	depth          int
	ignoreNextType bool
	ci             *cycleInfo
	cs             *ConfigState
}

// buildDefaultFormat recreates the original format string without precision
// and width information to pass in to fmt.Sprintf in the case of an
// unrecognized type.  Unless new types are added to the language, this
// function won't ever be called.
func (f *formatState) buildDefaultFormat() (format string) {
	buf := bytesBufferGet()
	defer bytesBufferPut(buf)
	buf.Write(percentBytes)

	for _, flag := range supportedFlags {
		if f.fs.Flag(int(flag)) {
			buf.WriteRune(flag)
		}
	}

	buf.WriteRune('v')
	return buf.String()
}

// constructOrigFormat recreates the original format string including precision
// and width information to pass along to the standard fmt package.  This allows
// automatic deferral of all format strings this package doesn't support.
func (f *formatState) constructOrigFormat(verb rune) (format string) {
	buf := bytesBufferGet()
	defer bytesBufferPut(buf)
	buf.Write(percentBytes)

	for _, flag := range supportedFlags {
		if f.fs.Flag(int(flag)) {
			buf.WriteRune(flag)
		}
	}

	if width, ok := f.fs.Width(); ok {
		buf.WriteString(strconv.Itoa(width))
	}

	if precision, ok := f.fs.Precision(); ok {
		buf.Write(precisionBytes)
		buf.WriteString(strconv.Itoa(precision))
	}

	buf.WriteRune(verb)
	return buf.String()
}

// unpackValue returns values inside of non-nil interfaces when possible and
// ensures that types for values which have been unpacked from an interface
// are displayed when the show types flag is also set.
// This is useful for data types like structs, arrays, slices, and maps which
// can contain varying types packed inside an interface.
func (f *formatState) unpackValue(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Interface {
		f.ignoreNextType = false
		if !v.IsNil() {
			v = v.Elem()
		}
	}
	return v
}

// formatPtr handles formatting of pointers by indirecting them as necessary.
func (f *formatState) formatPtr(v reflect.Value) {
	// Display nil if top level pointer is nil.
	showTypes := f.fs.Flag('#')
	if v.IsNil() && (!showTypes || f.ignoreNextType) {
		f.fs.Write(nilAngleBytes)
		return
	}

	// Figure out how many levels of indirection there are by derferencing
	// pointers and unpacking interfaces down the chain while detecting circular
	// references.
	ve := derefPtr(v, f.depth, f.ci)

	// Display type or indirection level depending on flags.
	if showTypes && !f.ignoreNextType {
		f.fs.Write(openParenBytes)
		for i := 0; i < f.ci.indirects; i++ {
			f.fs.Write(asteriskBytes)
		}
		io.WriteString(f.fs, ve.Type().String())
		f.fs.Write(closeParenBytes)
	} else {
		if f.ci.nilFound || f.ci.cycleFound {
			f.ci.indirects += strings.Count(ve.Type().String(), "*")
		}
		f.fs.Write(openAngleBytes)
		for i := 0; i < f.ci.indirects; i++ {
			io.WriteString(f.fs, "*")
		}
		f.fs.Write(closeAngleBytes)
	}

	// Display pointer information depending on flags.
	if f.fs.Flag('+') && (len(f.ci.pointerChain) > 0) {
		f.fs.Write(openParenBytes)
		for i, addr := range f.ci.pointerChain {
			if i > 0 {
				f.fs.Write(pointerChainBytes)
			}
			printHexPtr(f.fs, addr)
		}
		f.fs.Write(closeParenBytes)
	}

	// Display dereferenced value.
	switch {
	case f.ci.nilFound:
		f.fs.Write(nilAngleBytes)

	case f.ci.cycleFound:
		f.fs.Write(circularShortBytes)

	default:
		f.ignoreNextType = true
		f.format(ve)
	}
}

// format is the main workhorse for providing the Formatter interface.  It
// uses the passed reflect value to figure out what kind of object we are
// dealing with and formats it appropriately.  It is a recursive function,
// however circular data structures are detected and handled properly.
func (f *formatState) format(v reflect.Value) {
	// Handle invalid reflect values immediately.
	kind := v.Kind()
	if kind == reflect.Invalid {
		f.fs.Write(invalidAngleBytes)
		return
	}

	// Handle pointers specially.
	if kind == reflect.Ptr {
		f.formatPtr(v)
		return
	}

	// Print type information unless already handled elsewhere.
	if !f.ignoreNextType && f.fs.Flag('#') {
		f.fs.Write(openParenBytes)
		io.WriteString(f.fs, v.Type().String())
		f.fs.Write(closeParenBytes)
	}
	f.ignoreNextType = false

	printValue(f.fs, f, v, kind, f.cs)
}

// Format satisfies the fmt.Formatter interface. See NewFormatter for usage
// details.
func (f *formatState) Format(fs fmt.State, verb rune) {
	f.fs = fs

	// Use standard formatting for verbs that are not v.
	if verb != 'v' {
		format := f.constructOrigFormat(verb)
		fmt.Fprintf(fs, format, f.value)
		return
	}

	if f.value == nil {
		if fs.Flag('#') {
			fs.Write(interfaceBytes)
		}
		fs.Write(nilAngleBytes)
		return
	}

	f.ci = cycleInfoGet()
	defer func() {
		cycleInfoPut(f.ci)
		f.ci = nil
	}()
	f.format(reflect.ValueOf(f.value))
}

func (f *formatState) printArray(v reflect.Value) {
	f.fs.Write(openBracketBytes)
	f.depth++
	if (f.cs.MaxDepth != 0) && (f.depth > f.cs.MaxDepth) {
		f.fs.Write(maxShortBytes)
	} else {
		numEntries := v.Len()
		for i := 0; i < numEntries; i++ {
			if i > 0 {
				f.fs.Write(spaceBytes)
			}
			f.ignoreNextType = true
			f.format(f.unpackValue(v.Index(i)))
		}
	}
	f.depth--
	f.fs.Write(closeBracketBytes)
}

func (f *formatState) printString(v reflect.Value) {
	io.WriteString(f.fs, v.String())
}

func (f *formatState) printMap(v reflect.Value) {
	f.fs.Write(openMapBytes)
	f.depth++
	if (f.cs.MaxDepth != 0) && (f.depth > f.cs.MaxDepth) {
		f.fs.Write(maxShortBytes)
	} else {
		keys := v.MapKeys()
		if f.cs.SortKeys {
			sortValues(keys, f.cs)
		}
		for i, key := range keys {
			if i > 0 {
				f.fs.Write(spaceBytes)
			}
			f.ignoreNextType = true
			f.format(f.unpackValue(key))
			f.fs.Write(colonBytes)
			f.ignoreNextType = true
			f.format(f.unpackValue(v.MapIndex(key)))
		}
	}
	f.depth--
	f.fs.Write(closeMapBytes)
}

func (f *formatState) printStruct(v reflect.Value) {
	numFields := v.NumField()
	f.fs.Write(openBraceBytes)
	f.depth++
	if (f.cs.MaxDepth != 0) && (f.depth > f.cs.MaxDepth) {
		f.fs.Write(maxShortBytes)
	} else {
		vt := v.Type()
		for i := 0; i < numFields; i++ {
			if i > 0 {
				f.fs.Write(spaceBytes)
			}
			vtf := vt.Field(i)
			if f.fs.Flag('+') || f.fs.Flag('#') {
				io.WriteString(f.fs, vtf.Name)
				f.fs.Write(colonBytes)
			}
			f.format(f.unpackValue(v.Field(i)))
		}
	}
	f.depth--
	f.fs.Write(closeBraceBytes)
}

func (f *formatState) defaultFormat() string {
	return f.buildDefaultFormat()
}

// newFormatter is a helper function to consolidate the logic from the various
// public methods which take varying config states.
func newFormatter(cs *ConfigState, v interface{}) fmt.Formatter {
	var f formatState
	f.Reset(cs, v)
	return &f
}

// Reset resets the formatter state.
func (f *formatState) Reset(cs *ConfigState, v interface{}) {
	*f = formatState{value: v, cs: cs}
}

/*
NewFormatter returns a custom formatter that satisfies the fmt.Formatter
interface.  As a result, it integrates cleanly with standard fmt package
printing functions.  The formatter is useful for inline printing of smaller data
types similar to the standard %v format specifier.

The custom formatter only responds to the %v (most compact), %+v (adds pointer
addresses), %#v (adds types), or %#+v (adds types and pointer addresses) verb
combinations.  Any other verbs such as %x and %q will be sent to the the
standard fmt package for formatting.  In addition, the custom formatter ignores
the width and precision arguments (however they will still work on the format
specifiers not handled by the custom formatter).

Typically this function shouldn't be called directly.  It is much easier to make
use of the custom formatter by calling one of the convenience functions such as
Printf, Println, or Fprintf.
*/
func NewFormatter(v interface{}) fmt.Formatter {
	return newFormatter(&Config, v)
}

var formattersPool sync.Pool

func formattersPut(pv interface{}) { formattersPool.Put(pv) }
func formattersGet(cs *ConfigState, args []interface{}) (pv interface{}, formatters []interface{}) {
	pv = formattersPool.Get()
	if pv != nil {
		formatters = pv.([]interface{})
		if len(formatters) < len(args) {
			formattersPool.Put(pv)
			pv = nil
		}
	}
	if pv == nil {
		pv = make([]interface{}, len(args))
		formatters = pv.([]interface{})
		for i := 0; i < len(args); i++ {
			formatters[i] = new(formatState)
		}
	}
	if len(formatters) > len(args) {
		formatters = formatters[:len(args)]
	}
	for i, arg := range args {
		formatters[i].(*formatState).Reset(cs, arg)
	}
	return pv, formatters
}
