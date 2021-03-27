package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/go-playground/webhooks.v5/github"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

const (
	webhookPath     = "/wh"
	invoicePath     = "/invoiceraw"
	invoicePagePath = "/invoice"

	claimPath  = "/claim"
	amtkey     = "amt"
	issueidkey = "issue_id"
)

type WebhookHandler struct {
	is      *IssueService
	webhook *github.Webhook
	tmpl    *template.Template

	ipRange []string
}

func NewWebhookHandler(is *IssueService, secret string, ipRange []string) (*WebhookHandler, error) {
	tmpl, err := template.ParseFiles("./dist/invoice.html")
	if err != nil {
		return nil, err
	}
	webhook, err := github.New(github.Options.Secret(secret))
	if err != nil {
		return nil, err
	}
	return &WebhookHandler{is: is, webhook: webhook, ipRange: ipRange, tmpl: tmpl}, nil
}

func (wh *WebhookHandler) SetupIpaddress(ip string) {

}

type InvoiceResponse struct {
	Invoice string `json:"invoice"`
}

type InvoicePageData struct {
	Invoice string
}

func (wh *WebhookHandler) StartHandler(address string) error {
	router := httprouter.New()
	router.POST(webhookPath, wh.handleWebhook)
	router.POST(webhookPath+"/:lndconnect", wh.handleWebhook)

	router.GET(invoicePath, wh.handleInvoice)

	router.GET(invoicePagePath, wh.handleInvoicePage)

	router.ServeFiles("/static/*filepath", http.Dir("dist"))
	return http.ListenAndServe(address, router)
}
func (wh *WebhookHandler) handleInvoice(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	query := r.URL.Query()
	if len(query) != 2 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid input, require %s and %s", amtkey, issueidkey))
		return
	}
	amt := query.Get(amtkey)
	issueId := query.Get(issueidkey)
	if amt == "" || issueId == "" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid input, require %s and %s", amtkey, issueidkey))
		return
	}
	amtInt, err := strconv.Atoi(amt)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v", err))
		return
	}
	issueIdInt, err := strconv.Atoi(issueId)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v", err))
		return
	}
	invoice, err := wh.is.GetBountyInvoice(r.Context(), int64(issueIdInt), int64(amtInt))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v", err))
		return
	}
	writeOkResponse(w, &InvoiceResponse{Invoice: invoice})
}

func (wh *WebhookHandler) handleInvoicePage(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	//invoice, err := wh.getInvoice(r)
	//if err != nil {
	//	writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v",err))
	//	return
	//}
	data := InvoicePageData{Invoice: "gude"}
	err := wh.tmpl.Execute(w, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v", err))
		return
	}
}

func (wh *WebhookHandler) getInvoice(r *http.Request) (string, error) {
	query := r.URL.Query()
	if len(query) != 2 {
		return "", fmt.Errorf("invalid input, require %s and %s", amtkey, issueidkey)
	}
	amt := query.Get(amtkey)
	issueId := query.Get(issueidkey)
	if amt == "" || issueId == "" {
		return "", fmt.Errorf("invalid input, require %s and %s", amtkey, issueidkey)
	}
	amtInt, err := strconv.Atoi(amt)
	if err != nil {
		return "", fmt.Errorf("something went wrong %v", err)
	}
	issueIdInt, err := strconv.Atoi(issueId)
	if err != nil {
		return "", fmt.Errorf("something went wrong %v", err)
	}
	invoice, err := wh.is.GetBountyInvoice(r.Context(), int64(issueIdInt), int64(amtInt))
	if err != nil {
		return "", fmt.Errorf("something went wrong %v", err)
	}
	return invoice, nil
}

func writeOkResponse(w http.ResponseWriter, res interface{}) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("something went wrong %v", err))
	}
}

func writeError(w http.ResponseWriter, statuscode int, msg string) {
	w.WriteHeader(statuscode)
	w.Write([]byte(msg))
}

func (wh *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	okay, err := wh.checkIps(r)
	if err != nil || !okay {
		return
	}
	lndConnectString := ps.ByName("lndconnect")
	if lndConnectString != "" {
		query := r.URL.Query()
		lndConnectString = "lndconnect://" + lndConnectString + "?cert=" + query.Get("cert") + "&macaroon=" + query.Get("macaroon")
	}
	payload, err := wh.webhook.Parse(r, github.IssuesEvent, github.LabelEvent)
	if err != nil {
		if err == github.ErrEventNotFound {
			// ok event wasn;t one of the ones asked to be parsed
		}
	}
	switch payload.(type) {
	case github.IssuesPayload:
		issue := payload.(github.IssuesPayload)
		if issue.Action == "closed" {
			err = wh.is.CloseIssue(context.Background(), issue.Issue.ID)
			if err != nil {
				log.Printf("Error closing bounty issue %v", err)
				return
			}
		}
		if issue.Action != "labeled" && issue.Action != "reopened" {
			return
		}
		if !hasBountyLabel(issue) {
			return
		}
		names := strings.Split(issue.Repository.FullName, "/")
		bi, err := wh.is.AddBountyIssue(context.Background(), issue.Issue.ID, issue.Issue.URL, names[0], issue.Repository.Name, issue.Issue.Number, lndConnectString)

		if err != nil {
			log.Printf("Error adding bounty issue %v", err)
			return
		}
		log.Printf("Successfully added bounty issue %v", bi)
	case github.LabelPayload:
		label := payload.(github.LabelPayload)
		fmt.Printf("%v", label)
	case github.Label:
		label := payload.(github.Label)
		fmt.Printf("%v", label)
	}
}
func hasBountyLabel(issuePayload github.IssuesPayload) bool {
	for _, v := range issuePayload.Issue.Labels {
		if v.Name == "bounty" {
			return true
		}
	}
	return false
}

func (wh *WebhookHandler) checkIps(r *http.Request) (bool, error) {
	okay, err := checkRemoteIp(r.RemoteAddr, wh.ipRange)
	if okay {
		return true, nil
	}
	okay, err = checkRemoteIp(r.Header.Get("X-Forwarded-For"), wh.ipRange)
	if okay {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil

}

func checkRemoteIp(remoteAddress string, ipRange []string) (bool, error) {
	reqIp := net.ParseIP(strings.Split(remoteAddress, ":")[0])
	for _, v := range ipRange {
		_, ipnet, err := net.ParseCIDR(v)
		if err != nil {
			return false, err
		}
		if ipnet.Contains(reqIp) {
			return true, nil
		}
	}
	return false, nil
}
