package main

import (
	"context"
	"fmt"
	bbolt2 "github.com/coreos/bbolt"
	"github.com/google/go-github/v33/github"
	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd/lnrpc"
	config "github.com/sputn1ck/github-bounty"
	"github.com/sputn1ck/github-bounty/lnd"
	"github.com/sputn1ck/github-bounty/tracker"
	"golang.org/x/oauth2"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// flow:
// add webhook on github
// on add label bounty:
// comment with invoice link
// add to issue db
// on pay invoice
// add to issue db
// on close stop allowing payments to come in
func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shutdown := make(chan struct{})
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		defer close(shutdown)
		defer close(sigChan)

		select {
		case sig := <-sigChan:
			log.Printf("received signal: %v, release shutdown", sig)
			shutdown <- struct{}{}
		}
	}()

	cfg := config.DefaultConfig()
	parser := flags.NewParser(cfg, flags.Default)
	_, err := parser.Parse()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	// create admin lnd client
	cc, err := lnd.ConnectFromLndConnectWithTimeout(ctx, cfg.LndConnect, time.Second*10)
	if err != nil {
		return fmt.Errorf("connecting to lnd error: %v", err)
	}
	defer cc.Close()
	lndClient := lnrpc.NewLightningClient(cc)
	// create boltdb
	boltDb, err := bbolt2.Open(cfg.DbFilePath, 0600, nil)
	if err != nil {
		return fmt.Errorf("unable to open token db: %v", err)
	}

	issueStore, err := tracker.NewBountyIssueStore(boltDb)
	if err != nil {
		return fmt.Errorf("unable to create issue store: %v", err)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.GithubAccessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	meta, _, err := client.APIMeta(ctx)
	if err != nil {
		return err
	}
	githubClient := tracker.NewGithubService(cfg.HttpUrl, client)
	issueService := tracker.NewIssueService(cfg, issueStore, githubClient, lndClient)

	fmt.Printf("recovering invoices \n")
	err = issueService.RecoverPayments(ctx)
	if err != nil {
		return err
	}

	webhookHandler, err := tracker.NewWebhookHandler(cfg, issueService, meta.Hooks)
	if err != nil {
		return fmt.Errorf("error starting http handler %v", err)
	}
	go startHandler(webhookHandler, cfg.ListenAddress)
	<-shutdown
	return nil
}

func startHandler(webhookhandler *tracker.WebhookHandler, listenAddress string) {
	fmt.Printf("listening on %s \n", listenAddress)
	err := webhookhandler.StartHandler(listenAddress)
	if err != nil {
		log.Fatal(err)
	}
}
