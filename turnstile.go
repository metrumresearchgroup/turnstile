package turnstile

import (
	"sync/atomic"
	"time"
)

//Scalable is a definition of an interface supported as an operation for the Manager
type Scalable interface {
	Prepare(channels *ChannelMap)
	Work(channels *ChannelMap)
	Monitor(channels *ChannelMap)
	Cleanup(channels *ChannelMap)
	CancellationChannel() chan bool
}

//Manager is what we will create first. It's the struct that will contain the details of the operations, total operational status, and the results
type Manager struct {
	Iterations  uint64
	Operations  []Scalable
	Working     uint64
	Completed   uint64
	Errors      uint64
	ErrorList   []ConcurrentError
	Channels    ChannelMap
	Concurrency uint64
	Work        chan Scalable //Where the work is actually put on the channel for operation
}

//ConcurrentError is used to place onto channels to keep track of errors generated in concurrency
type ConcurrentError struct {
	RunIdentifier string //Should be string to allow for flexibility of names / guids
	Notes         string
	Error         error
}

//ChannelMap is a struct containing all of our interoperational channels.
type ChannelMap struct {
	Working   chan int
	Completed chan int
	Failed    chan int
	Errors    chan ConcurrentError
}

//NewManager takes a few configuration options and prepares your work manager
func NewManager(operations []Scalable, concurrency uint64) *Manager {
	m := Manager{
		Operations:  operations,
		Concurrency: concurrency,
		Iterations:  uint64(len(operations)),
	}

	m.Work = make(chan Scalable, m.Iterations)

	m.Channels = ChannelMap{
		Working:   make(chan int, m.Iterations),
		Completed: make(chan int, m.Iterations),
		Failed:    make(chan int, m.Iterations),
		Errors:    make(chan ConcurrentError, m.Iterations),
	}

	return &m
}

//Execute begins the operation
func (m *Manager) Execute() {
	//Start listening on ye olden channels for work
	go func() {
		for {
			if m.Working < m.Concurrency {
				select {
				case work := <-m.Work:
					go func() {
						//Needs to be buffered
						cancellationChannel := work.CancellationChannel()

						work.Prepare(&m.Channels)

						if isCancelled(cancellationChannel, &m.Channels){
							return
						}

						work.Work(&m.Channels)

						if isCancelled(cancellationChannel, &m.Channels){
							return
						}

						work.Monitor(&m.Channels)

						if isCancelled(cancellationChannel, &m.Channels){
							return
						}

						work.Cleanup(&m.Channels)

						//Process cancellation channels one last time to look
						//for failures in the cleanup phase.
						isCancelled(cancellationChannel, &m.Channels)
					}()
				}
			}

			//Breakcondition for the routine
			if m.IsComplete() {
				break
			}

			time.Sleep(1 * time.Millisecond)
		}
	}()

	//Separate goroutine to listen for results (non blocking)
	go func() {
		for {
			select {
			//Actually do the work off the channel. This should be buffered.
			case <-m.Channels.Working:
				atomic.AddUint64(&m.Working, 1)
			case <-m.Channels.Completed:
				//Increment Completed
				atomic.AddUint64(&m.Completed, 1)
				//Decrement Working
				atomic.AddUint64(&m.Working, ^uint64(0))
				if m.IsComplete() {
					break
				}

			case <-m.Channels.Failed:
				atomic.AddUint64(&m.Errors, 1)

				if m.IsComplete() {
					break
				}

			case e := <-m.Channels.Errors:
				m.ErrorList = append(m.ErrorList, e)
			}
		}
	}()

	//Iterate over the scalables and put them onto the queue

	for _, v := range m.Operations {
		m.Work <- v
	}
}

//IsComplete simply returns a bool indication as to whether or not work for the manager has been completed
func (m *Manager) IsComplete() bool {
	return m.Completed >= m.Iterations
}


func isCancelled(cancellationChannel chan bool, cm *ChannelMap) bool {
	select {
		case is := <- cancellationChannel:
			if is {
				cm.Failed <- 1
				cm.Completed <- 1
				return true
			}
		default:
			//Nothing
	}

	return false
}

func CancellationChannel() chan bool {
	return make(chan bool,1)
}
