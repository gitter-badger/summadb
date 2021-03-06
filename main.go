package main

import (
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docopt/docopt-go"
	"gopkg.in/tylerb/graceful.v1"

	db "github.com/fiatjaf/summadb/database"
	handle "github.com/fiatjaf/summadb/handle"
	settings "github.com/fiatjaf/summadb/settings"
)

func main() {
	usage := `SummaDB ` + settings.VERSION + `

Usage:
  summadb [--reset] [--debug] [--cors=<domains>] [--port=<port>] [--db=<dbfile>]

Options:
  -h --help        Show this screen.
  --version        Show version.
  --db=<dbfile>    The path of the underlying LevelDB [default: /tmp/summa.db]
  --port=<port>    Choose the port in which the HTTP server will listen [default: 5000]
  --cors=<domains> Specify a list of comma-separated domains [default: *]
  --reset          Before starting, erase all database contents and start from zero.
  --debug          Force debug. Overrides DEBUG and LOGLEVEL environment variable.
    `
	arguments, _ := docopt.Parse(usage, nil, true, settings.VERSION, false)
	settings.HandleArgs(arguments)

	log.WithFields(log.Fields{
		"DBFILE":       settings.DBFILE,
		"PORT":         settings.PORT,
		"CORS_ORIGINS": settings.CORS_ORIGINS,
		"LOGLEVEL":     settings.LOGLEVEL,
	}).Info("starting database server.")

	if reset, _ := arguments["--reset"]; reset != nil && reset.(bool) {
		db.Erase()
	}
	db.Start()

	handler := handle.BuildHandler()
	server := &graceful.Server{
		Timeout: 2 * time.Second,
		Server: &http.Server{
			Addr:    ":" + settings.PORT,
			Handler: handler,
		},
	}
	stop := server.StopChan()
	server.ListenAndServe()

	<-stop
	log.Info("Exiting...")
	db.End()
}
