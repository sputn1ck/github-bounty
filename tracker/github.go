package tracker

import (
	"context"
	"fmt"
	"github.com/google/go-github/v33/github"
	"strconv"
)


type GithubService struct {
	baseUrl string
	client *github.Client
}

func NewGithubService(baseUrl string, client *github.Client) *GithubService {
	return &GithubService{baseUrl: baseUrl, client: client}
}

func (g *GithubService) AddComment(ctx context.Context, bountyIssue *BountyIssue) (int64, error) {

	comment := &github.IssueComment{
		Body: g.getComment(bountyIssue.Bounty, bountyIssue.Id),
	}
	comment, _, err := g.client.Issues.CreateComment(ctx, bountyIssue.Owner, bountyIssue.Repo, int(bountyIssue.Number), comment)
	if err != nil {
		return 0,err
	}
	return *comment.ID, nil
}

func (g *GithubService) UpdateBountyComment(ctx context.Context, bountyIssue *BountyIssue) error {
	comment := &github.IssueComment{
		Body: g.getComment(bountyIssue.Bounty, bountyIssue.Id),
	}
	comment, _, err := g.client.Issues.EditComment(ctx, bountyIssue.Owner, bountyIssue.Repo, bountyIssue.CommentId, comment)
	if err != nil {
		return err
	}
	return nil
}

func (g *GithubService) CloseBountyComment(ctx context.Context, bountyIssue *BountyIssue) error {
	comment := &github.IssueComment{
		Body: g.closeComment(bountyIssue.Bounty),
	}
	comment, _, err := g.client.Issues.EditComment(ctx, bountyIssue.Owner, bountyIssue.Repo, bountyIssue.CommentId, comment)
	if err != nil {
		return err
	}
	return nil
}

func (gs GithubService) closeComment(totalAmt int64) *string {
	str := fmt.Sprintf("" +
		"Issue has been closed" +
		"\n \n Total bounty is %v", totalAmt)
	return &str
}

func (gs GithubService) getComment(totalAmt,id int64) *string{
	str := fmt.Sprintf("" +
		"Lightning Bounty has been activated" +
		"\n \n Current Bounty is %v \n \n" +
		"Increase the Bounty with %s", totalAmt, gs.getUrl(id))
	return &str
}

func (gs GithubService) getUrl(id int64) string {
	return fmt.Sprintf(gs.baseUrl+"/invoice?%s=%s&%s=100",issueidkey,strconv.Itoa(int(id)),amtkey)
}

