package buggy

import "errors"

// TODO: implement real caching here

// NotDone is an unimplemented stub.
func NotDone() error {
	panic("not implemented")
}

// discards ignores an error return value (unchecked-error).
func discards() {
	mightFail()
}

func mightFail() error {
	return errors.New("boom")
}
