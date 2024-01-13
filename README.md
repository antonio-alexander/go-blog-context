# go-blog-context (github.com/antonio-alexander/go-blog-context)

Although this conversation is a part of a much bigger conversation in regards to flow control, I think that we can talk [enough] about context from a practical perspective to give you the tools to apply them to more general concepts. Context (...in the context of Go), is a tool used _primarily_ to stop a process. The concept itself is very much like a [correlation id](https://github.com/stevejgordon/CorrelationId/blob/main/README.md) in that it's all or none: every process in a chain of execution must be aware of the correlation id to be able to communicate a log (or error) and attach it to that id.

At th end of this; you should:

- be able to describe what a context is from a practical perspective
- understand how to architect your functions to accept contexts
- use contexts to communicate data between layers of execution
- use a context to perform 1:N communication
- describe the use of contexts with fundamental concepts like REST/GRPC endpoints

## Bibliography

- [https://pkg.go.dev/context](https://pkg.go.dev/context)
- [https://go.dev/blog/context](https://go.dev/blog/context)
- [https://jwt.io/](https://jwt.io/)
- [https://en.wikipedia.org/wiki/Dependency_inversion_principle](https://en.wikipedia.org/wiki/Dependency_inversion_principle)

## TLDR; Too Long Didn't Read

Contexts are a flow control mechanism in Go that can be used to communicate to one or more listening processes that they should stop. Context can be configured to signal "stop" on execution of a cancel function or after a certain time period passes. Different contexts can be linked together such that if the parent context stops, all child contexts will also stop.

```go
package main

import (
    "context"
    "fmt"
    "time"
)

const keyCtx string = "foobar"

func main() {
    //create parent context with value
    ctxParent := context.WithValue(context.Background(), keyCtx, "duck")

    //create context that is "done" in 1s
    ctx, cancel := context.WithTimeout(ctxParent, time.Second)
    defer cancel()

    //print value with key
    fmt.Println(ctx.Value(keyCtx))

    //store the current time
    tNow := time.Now()

    //defer function to print time since tNow
    defer func() {
        fmt.Println(time.Since(tNow))
    }()

    //wait until the context is done
    <-ctx.Done()
}
```

Contexts are really simply, but architecturally they're pervasive, it's not as useful to use contexts in only one place and it's best when a context goes down through each layer of a given application.

## Why do I even need a context?

Go is a language that prides itself on being a box of hammers, nails and screw drivers. In general, if you need something, you can build it yourself. Context is similar to that; you don't need to use context, but using context makes it so much easier. But what's easy if you don't go through the trouble of trying to do it yourself first?

Context can do a couple of things, we'll try to do these on our own:

- communicate 1:N to stop; also known as a one-shot or single-use (i.e., you can't actively communicate more than once)
- allow polling to see if the 1:N has stopped
- statically communicate a value to downstream processes

Let's start with the first two, with a relatively simple proof of concept:

```go
package main

import (
    "fmt"
    "sync"
    "time"
)

func main() {
    var wg sync.WaitGroup

    //create two signal channels, one for starting and another
    // for stopping
    starter, stopper := make(chan struct{}), make(chan struct{})

    //create two go routines to show 1:N communication
    wg.Add(2)
    for i := 0; i < 2; i++ {
        go func(i int) {
            defer wg.Done()

            //create a ticker (to show polling)
            tCheck := time.NewTicker(time.Second)
            defer tCheck.Stop()

            //wait on starter (to synchronize go routines)
            <-starter
            for {
                select {
                case <-tCheck.C:
                    fmt.Printf("checking (%d)\n", i)
                case <-stopper:
                    fmt.Printf("stopped (%d)\n", i)
                    return
                }
            }
        }(i)
    }

    //close the starter to have the go routines
    // start their business logic
    close(starter)

    //let the go routines run for 10s
    <-time.After(10 * time.Second)

    //close the stopper to tell the go routines to stop
    // wait for all the go routines to return
    close(stopper)
    wg.Wait()
}
```

This is the expected output:

```log
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
checking (1)
checking (0)
stopped (0)
checking (1)
stopped (1)
```

There may be some generalswapping or alternate ordering of the logs, but for the most part, they'll be the same. Signal channels are useful/interesting because a side-effect of channels is that once you close them, they stop blocking; this specific functionality allows you to do a 1:N communication between different threads as long as you can handle the situation when the channel stops blocking.

> if you were curious, if you don't handle the situation the for loop will generally run as fast as possible, drive up CPU usage and otherwise run unexpectedly

Although it's not a direct example, you can also see how we controlled the stopper using time.After. I won't go through the trouble of showing you how you can statically communicate between processes because that's simple (you just pass a variable through); but you may find that it doesn't scale well and the use of a signal doesn't do it either (since signals are really bad at communicating values).

Knowing that you can use a stopper and have an idea about how you'd communicate statically between processes, I want you to ponder the following:

- what if you wanted one process to time out if the high-level stopper was closed or a certain amount of time passed? what if you wanted another process to stop when the time passed (but didn't care about the high-level stopper)?
- what if you needed to communicate static data through 10 layers? what if it wasn't just a scalar variable, but a host of values? what if those values were added by different layers?

And these questions aren't necessarily "for" context, but really to try to communicate how __convenient__ context is. Stoppers and some combination of magic can get you where you need to go, but context just does it so much better.

## Context and Architecture

Contexts are annoying because to implement them _correctly_ you have to put it in every layer and function you want to be able to apply flow control. To some degree, even integrating signals and contexts to co-exist is a pain. I mention context AND architecture because you may not be lucky enough to know if you'll use context at every layer, but if you don't pass it through at the _start_ it could be more difficult to implement it after. In addition, contexts are polarizing, you'll find that some APIs/libraries provide a context and non-context version of their functions and some that don't (forcing you to forgo flow control, or having to wait).

In general, within an archiecture built to support context, you should have it as one of the first inputs to every function that requires flow control. For example:

```go
type Test interface {
    Function(ctx context.Context, argOne string, argTwo int) error
}
```

And yes, it's totally possible (and reasonable) to store a correlation id inside a context:

```go
ctx, cancel := context.WithCancel(context.Background)
defer cancel()

ctx = context.WithValue(ctx, "correlation_id", "abc123")
```

It's a bit of a chore to get the correlation id out, but you probably only need it 20% of the time and you can always write a function to pull the value out of the context. More on this later. And this kind of leads us into the biggest weakness of contexts: "when you want to build your life around it".

Most people have a poor experience with contexts when they want to use it to do something more than flow control (i.e., use it to communicate static values). Contexts do this _really_ well but have the problem in that it obfuscates the data inside the context. When you look at a context in code, there's no way to know what values are stored in the context unless you look at the rest of the code. This is mostl an issue with supporting the code long term and less with context not being a good solution.

With that said, any time you implement contexts, there are two things you should always do:

- always, always, always defer cancel() for contexts that are not being shoved into a go routine
- if you're putting a context into a go routine, store the cancel() function and run it in the same code where you're doing the waitgroup.Wait()
- if you store data in a context, always use a key that's an unexported string type or localized to a specific layer of your code using an internal package

> if all layers using context implement localization of keys and/or some kind of interface/set of functions, you can avoid problems with overlapping keys

```go
package main

import (
    "context"
    "fmt"
    "time"
)

func main() {
    const keyCtx string = "foobar"

    //create context that is "done" in 1s
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    //use a localized key to store a value in the context
    ctx = context.WithValue(ctx, keyCtx, "duck")

    //print value with key
    fmt.Println(ctx.Value(keyCtx))

    //store the current time
    tNow := time.Now()

    //defer function to print time since tNow
    defer func() {
        fmt.Println(time.Since(tNow))
    }()

    //wait until the context is done
    <-ctx.Done()
}
```

> Context.Background() will remain valid and is never cancelled, although not equivalent to context.TODO() (something you shouldn't use outside of tests) for all intents and purposes it is.

You may be wondering about the pre-occupation with defer cancel() and it's to prevent context leaks. Context leaks are generally only a big issue if you're application is creating contexts as a part of its business logic. Context leaks generally mean that there's a context in-memory that's still functional but no-one is using it. This has implications for memory and cpu usage and will generally show up in long running applications as a CPU/memory leak. Practically, it means that when you stop your application, it's not cleaning up after itself and is is poor quality code from an academic perspective.

## Example: Handling REST Timeouts with Context

This isn't specific to REST, but it makes a _better_ practical example. One of the rough things about a REST is that users don't really like to wait too long (waiting is a bad user experience); so users will not only refresh/stop pages that load slowly, but browsers have a built in default timeout of one minute. So for REST endpoints, what happens when a timeout occurs or the user refreshes/cancels the loading of a page? Well the answer is it depends; but if you haven't considered this possibility, the endpoint probably just finishes and then encounters and internal error because no-one is listening on the other end.

There are legitimate reasons why an endpoint takes a long time to complete and although we could focus on that, the problem remains. Endpoints that continue to execute when no-one is listening waste memory and CPU that could be better spent on an endpoint with someone who _IS_ listening. With that said, the Go http server has the ability to provide a context which if the connection dies, will cancel itself. This can be useful when for processes that may timeout before they complete.

Below is a subset of the code in this repo (located at [./cmd/rest/main.go](./cmd/rest/main.go)):

```go
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
```

Once you run the example, you can attempt to connect to the webserver using the /timeout and /timeout/ctx endpoints to see the difference. Both endpoint will take a timeout query parameter to show how long to wait. You'll notice that if you hit the refresh button or stop loading on the timeout endpoint connected to timeout/ctx, it'll return almost immediately, while on the non ctx endpoint, it'll complete its execution.

Take a look around common APIs, you'll notice that some of the first party APIs (e.g., the database API) contains functions that use contexts and functions that don't.

## Example: Using Context to Communicate Auditing Information

In general, I _don't_ like this example, not because its impractical, but because I don't think the format of this repository will do it justice. I think this example is the best when the application stretches its legs: with fully functional (and sprawling) layers. Auditing can be complex; generally, the part of the application that can identify who is accessing the endpoint, isn't the part that writes to the database. Because these two parts (or layers) of the application generally aren't the same and in a lot of cases aren't even adjacent (or aware) of each other; communicating between the two can result in [inelegant] solutions like the following:

- data transfer objects (DTOs): the layer that knows the audting information would populate a DTO shared between the different layers with the auditing information (passing exposed, unused information between layers)
- useless arguments: passing arguments through the layers, this is generally bad because it often has to traverse a layer that is totally unassociated with the variable (generally a code smell)

> These solutions aren't bad, but they're not great; at scale or at least with developer's not aware of the nuance of these solutions, they can spiral out of control (and make the code more difficult to manage long term)

At a glance, this is the process from start to finish:

1. create a token with the claims having id and user_id fields (both strings) (and the symmetric signing key)
2. parse the claims and verify the access token (in the authorization header or a query parameter)
3. use the context from the endpoint and update it with the auditing information (e.g., a user id)
4. pass the context from the endpoint to the logic
5. pass the context from the logic to persistence/metadata
6. get the auditing data from the context and use to update the data in persistence/metadata

The code would look like the following:

```go
package rest_audit

import (
    "context"
    "flag"
    "fmt"
    "net/http"
    "os"
    "sync"

    "github.com/golang-jwt/jwt/v4"
)

type ctxKey string

const (
    keyCtxUserId ctxKey = "user_id"
    keyCtxId     ctxKey = "id"
)

type Claims struct {
    jwt.RegisteredClaims
    Id     string `json:"id"`
    UserId string `json:"user_id"`
}

func endpointToken(jwtKey string) func(http.ResponseWriter, *http.Request) {
    return func(writer http.ResponseWriter, request *http.Request) {
        token := request.Header.Get("authorization")
        if s := request.URL.Query().Get("token"); s != "" {
            token = s
        }
        claims := &Claims{}
        if _, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
            return []byte(jwtKey), nil
        }); err != nil {
            fmt.Printf("error (%s): %s", claims.Id, err.Error())
            writer.WriteHeader(http.StatusInternalServerError)
            if _, err := writer.Write([]byte(err.Error())); err != nil {
                fmt.Printf("error (%s): %s", claims.Id, err.Error())
            }
            return
        }
        ctx := context.WithValue(request.Context(), keyCtxUserId, claims.UserId)
        ctx = context.WithValue(ctx, keyCtxId, claims.Id)
        logicAuditing(ctx)
    }
}

func logicAuditing(ctx context.Context) {
    metaAuditing(ctx)
}

func metaAuditing(ctx context.Context) {
    id, userId := ctx.Value(keyCtxId), ctx.Value(keyCtxUserId)
    fmt.Printf("audit (%s); userId: %s\n", id, userId)
}

func Main(pwd string, args []string, envs map[string]string, osSignal chan os.Signal) error {
    var httpAddress, httpPort, jwtKey string
    var wg sync.WaitGroup
    var err error

    //get address/port from args
    cli := flag.NewFlagSet("", flag.ContinueOnError)
    cli.StringVar(&httpAddress, "address", "", "http address")
    cli.StringVar(&httpPort, "port", "8080", "http port")
    cli.StringVar(&jwtKey, "jwt_key", "secret", "jwt key")
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
    if _, ok := envs["JWT_KEY"]; ok {
        jwtKey = envs["JWT_KEY"]
    }

    //generate and create handle func, when connecting, it will use this port
    // indicate via console that the webserver is starting
    http.HandleFunc("/", endpointToken(jwtKey))
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
```

A valid token (at the time of writing) would be: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwidXNlcl9pZCI6ImRhNjY3ZGM0LTA1MWMtNDlmMC05NWUyLTQzMzQ2ZjliNzEyNiIsImlkIjoiNDBlZTlhZjctZDVkMS00YWU0LWExOGItYmM4NWZiOWJmOWM0IiwiaWF0IjoxNTE2MjM5MDIyfQ.4TTqETatyl19MdnjNAns6mb4ddNZiPFRIvRtZPqbM58

You can generate a signed token using [http://jwt.io](http://jwt.io)

```json
{
  "sub": "1234567890",
  "name": "John Doe",
  "user_id": "da667dc4-051c-49f0-95e2-43346f9b7126",
  "id": "40ee9af7-d5d1-4ae4-a18b-bc85fb9bf9c4",
  "iat": 1516239022
}
```

The code is relatively self explanatory but i'll hit the high points:

- use context to store user id and id is more elegant to functions and layers that don't necessarily need to know about the auditing information and get the inherent benefit of having a context for flow control
- claims are type safe and have known contracts that can be versioned (this is good for long term maintenance)

A curious thing will happen if you attempt to use bare strings with context.WithValue() and you have staticcheck or a linter (e.g., golangci-lint) enabled, you'll get a message: "should not use built-in type string as key for value; define your own type to avoid collisions (SA1029) (from go-staticcheck)". This is one of the things that makes Go a wonderful language (or at least a language with wonderful support). This is an architecture thing and is a simple way to say you should localize your keys for context values so layers that don't need to know can remain ignorant.

The concept of localization isn't a new thing, all languages have some concept of localization, but localization isn't just about where you instantiate a variable, but where and how it can be used.
