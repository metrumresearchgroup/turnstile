package turnstile

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type Meow struct {
	Number int
	Text   string
	Other  int
}

func (m Meow) Prepare(channels *ChannelMap) {
	println(m.Other)
	channels.Working <- 1
}

func (m Meow) Work(channels *ChannelMap) {
	rand.Seed(time.Now().UnixNano())
	random := rand.Intn(m.Number)
	morerandom := rand.Intn(2)
	fmt.Printf("Holding for %d seconds\n", random)

	if morerandom%2 == 0 {
		//Random error condition
		ce := ConcurrentError{
			RunIdentifier: random,
			Notes:         "A planned error occurred",
			Error:         errors.New("A miscellaneous error has teh occurred"),
		}

		channels.Failed <- 1
		channels.Errors <- ce
		return
	}

	time.Sleep(time.Duration(random) * time.Second)

	channels.Completed <- 1
}

func (m Meow) Monitor(channels *ChannelMap) {
	//Not used here, but if we wanted to monitor the actual work
	//in order to block (IE Watch items on the grid), this is where we could do it.
}

func (m Meow) Cleanup(channels *ChannelMap) {
	//Cleanup should be executable in other threads
	go func() {
		println(m.Text)
	}()
}

func TestNewManager(t *testing.T) {

	m := Meow{
		Number: 10,
		Text:   "I am a kitty cat, and I dance, dance, dance",
		Other:  5,
	}

	manager := NewManager(m, 500, 100)

	go manager.Execute()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

	var wg sync.WaitGroup

	wg.Add(1)

	go func(ctx context.Context, t *testing.T) {
		for {
			println(manager.Working)
			time.Sleep(2 * time.Second)

			if manager.Working > manager.Concurrency {
				t.Errorf("We're not effectively queuing. We have %d workers when we should only have %d", manager.Working, manager.Concurrency)
			}

			if manager.isComplete() {
				if len(manager.ErrorList) == 0 {
					t.Errorf("We should have a 50 percent probability of generating errors throughout this test")
				}
				wg.Done()
				return
			}

			//Look for cancellation
			go func(ctx context.Context) {
				select {
				case <-ctx.Done():
					t.Errorf("Operation timed out. Look for leaks")
					wg.Done()
					cancel()
					return
				}
			}(ctx)
		}
	}(ctx, t)

	wg.Wait()

}
