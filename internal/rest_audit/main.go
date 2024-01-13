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
