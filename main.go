// A sample main
// program that imports a package and calls a function from it.
package main		

import (
	"sample-k8-scheduler/scheduler" // Import the scheduler package
)		

func main() {
	scheduler.Start()
}