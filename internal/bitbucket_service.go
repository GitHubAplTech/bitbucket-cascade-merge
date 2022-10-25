package internal

import (
	"fmt"
	"github.com/ktrysmt/go-bitbucket"
	"golang.org/x/mod/semver"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

type BitbucketService struct {
	bitbucketClient *bitbucket.Client
	ReleaseBranchPrefix string
	DevelopmentBranchName string
}

func NewBitbucketService(bitbucketClient *bitbucket.Client,
	releaseBranchPrefix string,
	developmentBranchName string) *BitbucketService {

	return &BitbucketService{bitbucketClient,
		releaseBranchPrefix,
		developmentBranchName}
}

func (service *BitbucketService) OnMerge (request *PullRequestMergedPayload) error {
	log.Println("--------- START OnMerge ---------")

	// Only operate on release branches
	sourceBranchName := request.PullRequest.Source.Branch.Name
	destBranchName := request.PullRequest.Destination.Branch.Name
	authorId := request.PullRequest.Author.UUID

	log.Println("sourceBranchName", sourceBranchName)
	log.Println("destBranchName", destBranchName)
	log.Println("authorId", authorId)

	// NB!!! UNCOMMENT after testing!
	//if strings.HasPrefix(destBranchName, service.ReleaseBranchPrefix) {

		log.Println("Inside blk -> Only operate on release branches")

		repoName := request.Repository.Name
		log.Println("repoName (Name): ", repoName)
		repoOwner := request.Repository.Owner.Username
		log.Println("repoOwner (Username): ", repoOwner)
		if repoOwner == "" {
			repoOwner := request.Repository.Owner.UUID
			log.Println("repoOwner (UUID): ", repoOwner)
		}

		log.Println("Repository: ", repoName)
		log.Println("Source: ", sourceBranchName)
		log.Println("Destination: ", destBranchName)

		targets, err := service.GetBranches(repoName, repoOwner)
		if err != nil {
			return err
		}
		log.Println("Checking for internal targets: ", targets)
		nextTarget := service.NextTarget(destBranchName, targets)
		
		log.Println("Create PR -> Next Target: ", string(nextTarget))
		err = service.CreatePullRequest(destBranchName, nextTarget, repoName, repoOwner, authorId)
		if err != nil {
			return err
		}

		log.Println("--------- End Request Merged ---------")
	//}
	return nil
}

func (service *BitbucketService) TryMerge(dat *PullRequestMergedPayload) error {

	log.Println("--------- START TryMerge ---------")
	
	log.Println("dat.Repository.Name: ", dat.Repository.Name)
	log.Println("dat.Repository.Owner.Username: ", dat.Repository.Owner.Username)
	log.Println("dat.Repository.Owner.UUID: ", dat.Repository.Owner.UUID)
	log.Println("os.Getenv('BITBUCKET_USERNAME'): ", os.Getenv("BITBUCKET_USERNAME"))
	
	//err := service.DoApproveAndMerge(dat.Repository.Owner.Username, dat.Repository.Name)
	//err := service.DoApproveAndMerge(os.Getenv("BITBUCKET_USERNAME"), dat.Repository.Name)
	err := service.DoApproveAndMerge(dat.Repository.Owner.UUID, dat.Repository.Name)
	if err != nil {
		return err
	}
	//Try Merge (called from inside ApprovePullRequest)

	log.Println("--------- End TryMerge ---------")
	return nil
}

func (service *BitbucketService) NextTarget(oldDest string, cascadeTargets *[]string) string {
	targets := *cascadeTargets
	//Change release/YYYY.M.P --> vYYYY.M.P
	destination := strings.ReplaceAll(oldDest, service.ReleaseBranchPrefix, "v")

	//Change release/YYYY.M.P --> vYYYY.M.P
	for i, _ := range targets {
		targets[i] = strings.ReplaceAll(targets[i], service.ReleaseBranchPrefix, "v")
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return semver.Compare(targets[i], targets[j]) < 0
	})
	for _, target := range targets {
		if semver.Compare(destination, target) < 0 {
			return strings.ReplaceAll(target, "v", service.ReleaseBranchPrefix)
		}
	}
	return service.DevelopmentBranchName
}

/* ORIGINAL
func (service *BitbucketService) GetBranches(repoSlug string, repoOwner string) (*[]string, error) {

	log.Println("--------- START GetBranches ---------")

	var options bitbucket.RepositoryBranchOptions
	options.RepoSlug = repoSlug
	options.Owner = repoOwner
	options.Query = "name ~ " + service.ReleaseBranchPrefix
	options.Pagelen = 100

	branches, err := service.bitbucketClient.Repositories.Repository.ListBranches(&options)

	if err != nil {
		return nil, err
	}
	
	log.Println(" B4 Targets")

	targets := make([]string, len(branches.Branches))
	for i, branch := range branches.Branches {
	    log.Println("Targets -> branch.Name: ", branch.Name)
		targets[i] = branch.Name
	}
	return &targets, nil
}
*/

func (service *BitbucketService) GetBranches(repoSlug string, repoOwner string) (*[]string, error) {

	log.Println("--------- START GetBranches ---------")
/*
	var options bitbucket.RepositoryBranchOptions
	options.RepoSlug = repoSlug
	options.Owner = repoOwner
	options.Query = "name ~ " + service.ReleaseBranchPrefix
	options.Pagelen = 100

	branches, err := service.bitbucketClient.Repositories.Repository.ListBranches(&options)
*/
	//Original
    //url := service.bitbucketClient.GetApiBaseURL() + "/repositories/" + repoOwner + "/" + repoSlug + "/refs/branches"
	
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_PASSWORD")

	url := service.bitbucketClient.GetApiBaseURL() + "/repositories/" + username + "/" + repoSlug + "/refs/branches"
	log.Println(string(url))
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	response, err := service.bitbucketClient.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	branches, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	log.Println(string(branches))

	/* Working example for reference
	options := bitbucket.PullRequestsOptions{
		Owner:    repoOwner,
		RepoSlug: repoName,
		ID:       pullRequestId,
	}
	_, err := service.bitbucketClient.Repositories.PullRequests.Merge(&options)
	*/

	log.Println(" B4 Targets")
	/*
	targets := make([]string, len(branches.values))
	for i, branch := range branches.values {
		log.Println("Targets -> branch.Name: ", branch.Name)
		targets[i] = branch.Name
	}
	*/
	log.Println("--------- End GetBranches ---------")
	
	//return &targets, nil
	
	return nil, nil
}

func (service *BitbucketService) PullRequestExists(repoName string, repoOwner string, source string, destination string) (bool, error) {

	log.Println("--------- START PullRequestExists ---------")

	options := bitbucket.PullRequestsOptions{
		Owner:             repoOwner,
		RepoSlug:          repoName,
		Query:             "destination.branch.name = \"" + destination + "\" AND source.branch.name=\"" + source + "\"",
	}
	resp, err := service.bitbucketClient.Repositories.PullRequests.Gets(&options)
	if err != nil {
		return false, nil
	}
	pullRequests := resp.(map[string]interface{})
	log.Println("--------- End PullRequestExists ---------")
	
	return len(pullRequests["values"].([]interface{})) > 0, nil
}

func (service *BitbucketService) CreatePullRequest(src string, dest string, repoName string, repoOwner string, reviewer string) error {

	log.Println("--------- START CreatePullRequest ---------")

	exists, err := service.PullRequestExists(repoName, repoOwner, src, dest)

	if err != nil {
		return err
	}

	if exists {
		log.Println("Skipping creation. Pull Request Exists: ", src, " -> ", dest)
		return nil
	}

	options := bitbucket.PullRequestsOptions{
		ID:                "",
		CommentID:         "",
		Owner:             repoOwner,
		RepoSlug:          repoName,
		Title:             "#AutomaticCascade " + src + " -> " + dest,
		Description:       "#AutomaticCascade " + src + " -> " + dest+", this branch will automatically be merged on " +
			"successful build result+approval",
		CloseSourceBranch: false,
		SourceBranch:      src,
		SourceRepository:  "",
		DestinationBranch: dest,
		DestinationCommit: "",
		Message:           "",
		Reviewers:         []string{reviewer},
		States:            nil,
		Query:             "",
		Sort:              "",
	}

	_, err = service.bitbucketClient.Repositories.PullRequests.Create(&options)

	log.Println("--------- End CreatePullRequest ---------")
	return err
}

func (service *BitbucketService) DoApproveAndMerge(repoOwner string, repoName string) error {
	log.Println("--------- START DoApproveAndMerge ---------")

	log.Println("Set options...")

	options := bitbucket.PullRequestsOptions{
		Owner:             repoOwner,
		RepoSlug:          repoName,
		Query:             "title ~ \"#AutomaticCascade\" AND state = \"OPEN\"",
		States: 		   []string{"OPEN"},
	}
	log.Println("B4 GET pullRequests...")
	resp, err := service.bitbucketClient.Repositories.PullRequests.Gets(&options)
	if err != nil {
		return err
	}
	
	log.Println("B4 looping through pullRequests...")

	pullRequests := resp.(map[string]interface{})

	for _, pr := range pullRequests["values"].([]interface{}) {
		prUnwrapped := pr.(map[string]interface{})
		log.Println("ID: ",  prUnwrapped["id"])
		log.Println("Title: ", prUnwrapped["title"])
		log.Println("Trying to Auto Approve...")
		err = service.ApprovePullRequest(repoOwner, repoName, fmt.Sprintf("%v", prUnwrapped["id"]))
		if err != nil {
			return err
		}
	}

	log.Println("--------- End DoApproveAndMerge ---------")

	return nil
}

// HACK: There isn't an API method in the Bitbucket API Library to do pull request
// approval. Hacking together one here.
func (service *BitbucketService) ApprovePullRequest(repoOwner string, repoName string, pullRequestId string) error {
	log.Println("--------- START ApprovePullRequest ---------")

	url := service.bitbucketClient.GetApiBaseURL() + "/repositories/" + repoOwner + "/" + repoName + "/pullrequests/" + pullRequestId + "/approve"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_PASSWORD")
	req.SetBasicAuth(username, password)
	response, err := service.bitbucketClient.HttpClient.Do(req)
	if err != nil {
		return err
	}
	buf, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	log.Println(string(buf))

	//Try merge
	/* Uncomment to activate auto-merge!
	log.Println("Trying to Auto Merge...")
	err = service.MergePullRequest(repoOwner, repoName, pullRequestId)
	if err != nil {
		return err
	}
	*/
	log.Println("--------- End ApprovePullRequest ---------")
	return nil
}

func (service *BitbucketService) MergePullRequest(repoOwner string, repoName string, pullRequestId string) error {
	log.Println("--------- START MergePullRequest ---------")

	options := bitbucket.PullRequestsOptions{
		Owner:    repoOwner,
		RepoSlug: repoName,
		ID:       pullRequestId,
	}
	_, err := service.bitbucketClient.Repositories.PullRequests.Merge(&options)
	log.Println("--------- End MergePullRequest ---------")

	return err
}