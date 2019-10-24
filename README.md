# Turnstile -> Concurrency Limiter and Process Manager

Turnstile is a framework for managing phase-oriented operations in a concurrent fashion. By phase oriented, we mean that an operation has four defined phases, some of which may actually do nothing:

* Prepare - Prepation for the work. IE Pulling globs down, formatting data etc.
* Work - The phase in which actual computational work is done
* Monitor - A phase for monitoring work data. Valuable if your work is offloaded to a third party like SGE or SLURM. Monitoring the job to wait for its completion allows for jobs to block and still effectively rate limit without overloading your 3rd party software.
* Cleanup - A phase for removing temporary files, sanitizing the environment, etc. In theory this is fine to execute purely as a gorutine as it should not block other operations unless your workflow dictates it. 

Each of these phases represents a method within the Scalable interface, which will take a pointer to a ChannelMap, which contains all channels necessary to safely operate. Turnstile is meant to be used in isolation within a Go application to minimize need for external systems.

## Channels

In each phase, you'll have access to the following channels. Without interactions with these channels, there can be no real rate limiting or concurrency management. 

 * Working - `int` - This is an atomically incrementing / decrementing channel for the managers "Working" value. This indicates how many goroutines are actually presently doing the work phase. This is best fired at the end of the prepare phase as a defer action. 
 * Completed - `int` - This indicates how many of the iterations defined for the manager have completed. This is best implemented at the end of the work phase or the monitor phase depending on your needs. 
 * Failed - `int` - This channel indicates scenarios during your process in which errors have occurred. This allows you (on err) to notify the manager a failure has occurred
 * Errors - `ConcurrentError` If you do encounter an error, it would make sense to collect and summarize that information. You can build a ConcurrentError struct (Which contains identifiers, notes, and the error) and put it onto this channel for it to get added back to the Manager's `ErrorList` property for inspection later. 