package tracker

import (
	"context"
	"fmt"
	"github.com/google/go-github/v33/github"
	"strconv"
)

type GithubService struct {
	baseUrl string
	client  *github.Client
}

func NewGithubService(baseUrl string, client *github.Client) *GithubService {
	return &GithubService{baseUrl: baseUrl, client: client}
}

func (g *GithubService) AddComment(ctx context.Context, bountyIssue *BountyIssue) (int64, error) {

	comment := &github.IssueComment{
		Body: g.getComment(bountyIssue),
	}
	comment, _, err := g.client.Issues.CreateComment(ctx, bountyIssue.Owner, bountyIssue.Repo, int(bountyIssue.Number), comment)
	if err != nil {
		return 0, err
	}
	return *comment.ID, nil
}

func (g *GithubService) UpdateBountyComment(ctx context.Context, bountyIssue *BountyIssue) error {
	comment := &github.IssueComment{
		Body: g.getComment(bountyIssue),
	}
	comment, _, err := g.client.Issues.EditComment(ctx, bountyIssue.Owner, bountyIssue.Repo, bountyIssue.CommentId, comment)
	if err != nil {
		return err
	}
	return nil
}

func (g *GithubService) CloseBountyComment(ctx context.Context, bountyIssue *BountyIssue) error {
	comment := &github.IssueComment{
		Body: g.closeComment(bountyIssue),
	}
	comment, _, err := g.client.Issues.EditComment(ctx, bountyIssue.Owner, bountyIssue.Repo, bountyIssue.CommentId, comment)
	if err != nil {
		return err
	}
	return nil
}

func (gs GithubService) closeComment(bountyIssue *BountyIssue) *string {
	str := fmt.Sprintf(""+
		"Issue has been closed"+
		"\n \n Total bounty for %s was %v", bountyIssue.Pubkey, bountyIssue.Bounty)
	return &str
}

func (gs GithubService) getComment(bountyIssue *BountyIssue) *string {
	str := fmt.Sprintf(""+
		"Lightning Bounty is active"+
		"\n \n Benefactor: %s \n \n"+
		"\n \n Current Bounty is %v from %v payments \n \n"+
		"Donate Bounty with %s", bountyIssue.Pubkey, bountyIssue.Bounty, bountyIssue.TotalPayments, gs.getUrl(bountyIssue.Id))
	return &str
}

func (gs GithubService) getUrl(id int64) string {
	return fmt.Sprintf(gs.baseUrl+"/invoice?%s=%s&%s=100", issueidkey, strconv.Itoa(int(id)), amtkey)
}
