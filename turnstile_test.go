package turnstile

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type Meow struct {
	Number int
	Text   string
	Other  int
	Cancel chan bool
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
			RunIdentifier: strconv.Itoa(random),
			Notes:         "A planned error occurred",
			Error:         errors.New("A miscellaneous error has teh occurred"),
		}

		channels.Failed <- 1
		channels.Errors <- ce
		channels.Completed <- 1
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

type Woof struct {
	Number int
	Text   string
	Other  int
	Cancel chan bool
}

func (m Meow) CancellationChannel() chan bool {
	m.Cancel = make(chan bool)
	return m.Cancel
}

func (m Woof) CancellationChannel() chan bool {
	m.Cancel = make(chan bool)
	return m.Cancel
}

func (m Woof) Prepare(channels *ChannelMap) {
	println(m.Other)
	channels.Working <- 1
}

func (m Woof) Work(channels *ChannelMap) {
	//Automatically fails
	channels.Failed <- 1
	channels.Errors <- ConcurrentError{
		RunIdentifier: "woof",
		Notes:         "Broke",
		Error:         errors.New("Broke"),
	}
	channels.Completed <- 1
}

func (m Woof) Monitor(channels *ChannelMap) {
	//Not used here, but if we wanted to monitor the actual work
	//in order to block (IE Watch items on the grid), this is where we could do it.
}

func (m Woof) Cleanup(channels *ChannelMap) {
	//Cleanup should be executable in other threads
	go func() {
		println(m.Text)
	}()
}

func TestNewManager(t *testing.T) {

	var operations []Scalable
	iterations := 25

	for i := 0; i < iterations; i++ {
		operations = append(operations, Meow{
			Number: 10,
			Text:   "I am a kitty cat, and I dance, dance, dance",
			Other:  5,
		})
	}

	manager := NewManager(operations, uint64(10))

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

			if manager.IsComplete() {
				assert.GreaterOrEqual(t, len(manager.ErrorList), 1)     //At least one error should have occurred
				assert.Equal(t, uint64(iterations), manager.Completed)  //Verify the total execution count matches iterations
				assert.Equal(t, uint64(iterations), manager.Iterations) //Verify iterations matches what we supplied to the manager
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

func TestErrors(t *testing.T) {

	var operations []Scalable
	iterations := 25

	for i := 0; i < iterations; i++ {
		operations = append(operations, Woof{
			Number: 10,
			Text:   "I am a doggy dog, and I dance, dance, dance",
			Other:  5,
		})
	}

	manager := NewManager(operations, uint64(10))

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

			if manager.IsComplete() {
				assert.GreaterOrEqual(t, len(manager.ErrorList), 1)     //At least one error should have occurred
				assert.Equal(t, uint64(iterations), manager.Completed)  //Verify the total execution count matches iterations
				assert.Equal(t, uint64(iterations), manager.Iterations) //Verify iterations matches what we supplied to the manager
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

	for !manager.IsComplete() {
		log.Printf("Manager has %d working, %d errors, %d completed", manager.Working, manager.Errors, manager.Completed)
	}

	wg.Wait()

}

type canceller struct {
	Cancel chan bool
}

func (c canceller) Prepare(ch *ChannelMap){
	//Cancel
	ch.Working <- 1
	c.Cancel <- true //The counter should remain 0 at this point
	println("Cancelled the thing")
}

func (c canceller) Work(ch *ChannelMap){
	counter++
	println("working")
}

func (c canceller) Monitor(ch *ChannelMap){
	counter++
	println("Monitoring")
}

func (c canceller) Cleanup( ch *ChannelMap){
	counter++
	println("Cleanup")
}

func (c canceller) CancellationChannel() chan bool {
	return c.Cancel
}

var counter int


func TestCancellationOfFuturePhasesEarly(t *testing.T) {
	var operations []Scalable
	counter = 0

	canceller := canceller{
		Cancel: CancellationChannel(),
	}

	//Add it to work list
	operations = append(operations,canceller)

	manager := NewManager(operations, uint64(10))

	go manager.Execute()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

	ctx.Done()

	var wg sync.WaitGroup

	wg.Add(1)

	go func(ctx context.Context, t *testing.T) {
		for {
			println(manager.Working)
			time.Sleep(2 * time.Second)


			if manager.IsComplete() {
				assert.Equal(t,0,counter)
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

	for !manager.IsComplete() {
		log.Printf("Manager has %d working, %d errors, %d completed", manager.Working, manager.Errors, manager.Completed)
	}

	wg.Wait()
}


type midCanceller struct {
	Cancel chan bool
}

func (c midCanceller) Prepare(ch *ChannelMap){
	//Cancel
	ch.Working <- 1
	counter++
	println("preparation")
}

func (c midCanceller) Work(ch *ChannelMap){
	counter++
	println("working")
}

func (c midCanceller) Monitor(ch *ChannelMap){
	c.Cancel <- true
	println("Cancelled at monitoring")
}

func (c midCanceller) Cleanup( ch *ChannelMap){
	counter++
	println("Cleanup")
}

func (c midCanceller) CancellationChannel() chan bool {
	return c.Cancel
}


func TestCancellationOfFuturePhasesMiddle(t *testing.T) {
	counter = 0
	var operations []Scalable

	canceller := midCanceller{
		Cancel: CancellationChannel(),
	}

	//Add it to work list
	operations = append(operations,canceller)

	manager := NewManager(operations, uint64(10))

	go manager.Execute()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

	ctx.Done()

	var wg sync.WaitGroup

	wg.Add(1)

	go func(ctx context.Context, t *testing.T) {
		for {
			println(manager.Working)
			time.Sleep(2 * time.Second)


			if manager.IsComplete() {
				assert.Equal(t,2,counter)
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

	for !manager.IsComplete() {
		log.Printf("Manager has %d working, %d errors, %d completed", manager.Working, manager.Errors, manager.Completed)
	}

	wg.Wait()
}
