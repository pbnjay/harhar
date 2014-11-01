harhar
======

Simple, transparent HAR logging for Go code using the http.Client interface.
For existing code that already uses the `net/http` package, updating it to
produce HAR logs is typically only 2 lines of code.

Getting Started
---------------

First, convert your existing http.Client instance (or http.DefaultClient) to
a harhar.Client:

	// before
	webClient := &http.Client{}

	// after
	httpClient := &http.Client{}
	webClient := harhar.NewClient(httpClient)

Then, whenever you're ready to generate the HAR output, call WriteLog:

	webClient.WriteLog("output.har")

That's it! harhar.Client implements all the same methods as http.Client, so no
other code will need to be changed. However, if you set Timeouts, Cookies, etc.
dynamically then you will want to retain a copy of the wrapped http.Client.
harhar.Client only stores the pointer, so changes to the underlying http.Client
will be used immediately.

Optional periodic logging
-------------------------

To dynamically enable or disable HAR logging, code can use harhar.ClientInterface
to represent either an http.Client or harhar.Client. The included `harhar` example
command shows one way to use this interface. When using this interface, you can
write logs (if enabled) by using this simple block of code:

	if harCli, ok := myClient.(*harhar.Client); ok {
		harCli.WriteLog("output.har")
	}

When combined with a long-running process, the interface makes it possible to
toggle logging off and on, and periodically write to disk throughout a processes
lifetime. An example is the following (never-ending) goroutine:

	go func(){
		for _ = range time.Tick(time.Minute*5) {
			if harCli, ok := myClient.(*harhar.Client); ok {
				sz, err := harCli.WriteLog("output.har")
				if err!=nil {
					log.Println("error writing .har log:", err)
				} else {
					log.Printf("wrote .har log (%.1fkb)\n", float64(sz)/1024.0)
				}
			}
		}
	}()

Note that when logging is enabled, harhar memory usage can grow pretty quickly,
especially if Responses are large. If you don't want to disable logging in code
when output size grows too large, you should at least display it so that users
can decide to stop before the OOM killer comes to play.
