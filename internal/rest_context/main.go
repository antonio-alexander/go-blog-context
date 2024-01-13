package rest_context

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

func generateId() string {
	return uuid.Must(uuid.NewRandom()).String()
}

func endpointTimeout(writer http.ResponseWriter, request *http.Request) {
	id := generateId()
	tNow, timeout := time.Now(), time.Minute
	if s := request.URL.Query().Get("timeout"); s != "" {
		i, _ := strconv.Atoi(s)
		timeout = time.Duration(i) * time.Second
	}
	fmt.Printf("%s timeout: %v\n", id, timeout)
	<-time.After(timeout)
	fmt.Printf("%s completed\n", id)
	if _, err := fmt.Fprintf(writer, "%s: %v\n", id, time.Since(tNow)); err != nil {
		fmt.Printf("error (%s): %s", id, err.Error())
	}
}

func endpointTimeoutCtx(writer http.ResponseWriter, request *http.Request) {
	id := generateId()
	tNow, timeout := time.Now(), time.Minute
	if s := request.URL.Query().Get("timeout"); s != "" {
		i, _ := strconv.Atoi(s)
		timeout = time.Duration(i) * time.Second
	}
	fmt.Printf("%s timeout: %v\n", id, timeout)
	select {
	case <-request.Context().Done():
		fmt.Printf("%s cancelled via ctx: %v\n", id, time.Since(tNow))
		return
	case <-time.After(timeout):
		fmt.Printf("%s completed\n", id)
	}
	if _, err := fmt.Fprintf(writer, "%s: %v\n", id, time.Since(tNow)); err != nil {
		fmt.Printf("error (%s): %s", id, err.Error())
	}
}

func Main(pwd string, args []string, envs map[string]string, osSignal chan os.Signal) error {
	var httpAddress, httpPort string
	var wg sync.WaitGroup
	var err error

	//get address/port from args
	cli := flag.NewFlagSet("", flag.ContinueOnError)
	cli.StringVar(&httpAddress, "address", "", "http address")
	cli.StringVar(&httpPort, "port", "8080", "http port")
	if err := cli.Parse(args); err != nil {
		return err
	}

	//get address/port from env (overrides args)
	if _, ok := envs["HTTP_PORT"]; ok {
		httpPort = envs["HTTP_PORT"]
	}
	if _, ok := envs["HTTP_ADDRESS"]; ok {
		httpAddress = envs["HTTP_ADDRESS"]
	}

	//generate and create handle func, when connecting, it will use this port
	// indicate via console that the webserver is starting
	http.HandleFunc("/", endpointTimeout)
	http.HandleFunc("/ctx", endpointTimeoutCtx)
	server := &http.Server{
		Addr:    httpAddress + ":" + httpPort,
		Handler: nil,
	}
	fmt.Printf("starting web server on %s:%s\n", httpAddress, httpPort)
	stopped := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stopped)

		if err = server.ListenAndServe(); err != nil {
			return
		}
	}()
	select {
	case <-stopped:
	case <-osSignal:
		err = server.Close()
	}
	wg.Wait()
	return err
}
