package buffer

import (
	"errors"
	"sync"
	"time"
)

type Outbound struct {
	val int
	err error
	*sync.Cond
}

func NewOutbound(size int) *Outbound {
	return &Outbound{val: size, Cond: sync.NewCond(new(sync.Mutex))}
}

// wakeMe is a hack introduced to address a deadlock in 'Decrement'. The muxado
// code proved too complex for me to figure out the actual issue, but it
// ultimately boils down to something not calling 'Outbound.SetError' when the
// muxado stream or session gets closed. Most likely this is due to SetDeadline,
// where some write or call returns an error because of a conn timeout, and then
// the error is not handled correctly, the stream closes but 'SetError' does not
// get called, or perhaps even 'SetError' is called with a nil input?
//
// Regardless, we can prevent this deadlock by waiting some reasonable amount of
// time and then calling 'wakeMe'. 'wakeMe' can be interrupted by a call to
// 'Broadcast'. If wakeMe is forcefully woken, Outbound.SetError is called,
// compensating for the missed SetError wherever else.
func (b *Outbound) wakeMe(wakeChan chan struct{}) {
	select {
	case <-wakeChan:
		// Success, no need to trigger an error.
	case <-time.After(time.Minute*30):
		// A write call has been stuck on 'b.Wait()' for at least 30 minutes.
		// Set a timeout error so that it gets unstuck and returns, freeing up a
		// system thread.
		b.L.Lock()
		b.err = errors.New("Muxado error: global timeout")
		b.Broadcast()
		b.L.Unlock()
	}
}

func (b *Outbound) Increment(inc int) {
	b.L.Lock()
	b.val += inc
	b.Broadcast()
	b.L.Unlock()
}

func (b *Outbound) SetError(err error) {
	b.L.Lock()
	b.err = err
	b.Broadcast()
	b.L.Unlock()
}

func (b *Outbound) Decrement(dec int) (ret int, err error) {
	if dec == 0 {
		return
	}

	b.L.Lock()
	for {
		if b.err != nil {
			err = b.err
			break
		}

		if b.val > 0 {
			if dec > b.val {
				ret = b.val
				b.val = 0
				break
			} else {
				b.val -= dec
				ret = dec
				break
			}
		} else {
			wakeChan := make(chan struct{})
			go b.wakeMe(wakeChan)
			b.Wait()
			close(wakeChan)
		}
	}
	b.L.Unlock()
	return
}
