package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/ktrysmt/go-bitbucket"
	"golang.org/x/mod/semver"
)

type BitbucketService struct {
	bitbucketClient       *bitbucket.Client
	ReleaseBranchPrefix   string
	DevelopmentBranchName string
}

func NewBitbucketService(bitbucketClient *bitbucket.Client,
	releaseBranchPrefix string,
	developmentBranchName string) *BitbucketService {

	return &BitbucketService{bitbucketClient,
		releaseBranchPrefix,
		developmentBranchName}
}

// PrettyPrint to print struct in a readable way
func (service *BitbucketService) PrettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

func (service *BitbucketService) OnMerge(request *PullRequestMergedPayload) error {
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

	//targets, err := service.GetBranches(repoName, repoOwner)
	targets, err := service.GetBranches(repoName, request.Repository.Owner.UUID)

	if err != nil {
		return err
	}
	log.Println("Checking for internal targets: ", targets)

	nextTarget := service.NextTarget(destBranchName, targets)

	log.Println("Call Create PR -> Next Target: ", string(nextTarget))

	//err = service.CreatePullRequest(destBranchName, nextTarget, repoName, repoOwner, authorId)
	err = service.CreatePullRequest(destBranchName, nextTarget, repoName, request.Repository.Owner.UUID, authorId)
	if err != nil {
		return err
	}

	//}

	log.Println("--------- End OnMerge ---------")
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

/*
//ORIGINAL
func (service *BitbucketService) GetBranches(repoSlug string, repoOwner string) (*[]string, error) {

	log.Println("--------- START GetBranches ---------")

	var options bitbucket.RepositoryBranchOptions
	options.RepoSlug = repoSlug
	options.Owner = repoOwner
	options.Query = "name ~ " + service.ReleaseBranchPrefix
	options.Pagelen = 100

	branches, err := service.bitbucketClient.Repositories.Repository.ListBranches(&options)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	log.Println(" B4 Targets")

	targets := make([]string, len(branches.Branches))
	for i, branch := range branches.Branches {
	    log.Println("Targets -> branch.Name: ", branch.Name)
		targets[i] = branch.Name
	}
	log.Println("--------- End GetBranches ---------")

	return &targets, nil
}
*/

// My hacked version (ListBranches no longer supported?)
func (service *BitbucketService) GetBranches(repoSlug string, repoOwner string) (*[]string, error) {

	log.Println("--------- START GetBranches ---------")
	log.Println("repoOwner: ", repoOwner)
	log.Println("repoSlug: ", repoSlug)

	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_PASSWORD")

	url := service.bitbucketClient.GetApiBaseURL() + "/repositories/" + username + "/" + repoSlug + "/refs/branches"
	log.Println(string(url))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	response, err := service.bitbucketClient.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	//Enable for debugging
	//log.Println("B4 Targets -> Branches resp body: ", string(body))

	var result BranchesPayload

	if err := json.Unmarshal(body, &result); err != nil { // Parse []byte to go struct pointer
		log.Println("Can not unmarshal JSON")
		log.Fatal(err)
	}

	//Loop through the data
	targets := make([]string, len(result.Values))
	for i, branch := range result.Values {
		log.Println("Targets -> branch.Name: ", branch.Name)
		//TODO: Add only include if "name ~ " + service.ReleaseBranchPrefix
		targets[i] = branch.Name
	}

	log.Println("--------- End GetBranches ---------")

	//NB!!! Temp!!!
	//return nil, nil
	return &targets, nil
}

func (service *BitbucketService) PullRequestExists(repoName string, repoOwner string, source string, destination string) (bool, error) {

	log.Println("--------- START PullRequestExists ---------")

	log.Println("Set options...")
	log.Println("repoOwner: ", repoOwner)
	log.Println("repoName: ", repoName)

	options := bitbucket.PullRequestsOptions{
		Owner:    repoOwner,
		RepoSlug: repoName,
		Query:    "destination.branch.name = \"" + destination + "\" AND source.branch.name=\"" + source + "\"",
	}

	log.Println("B4 GET pullRequests...")

	resp, err := service.bitbucketClient.Repositories.PullRequests.Gets(&options)
	if err != nil {
		log.Fatal(err)
		return false, nil
	}

	pullRequests := resp.(map[string]interface{})
	log.Println("Pull Req exists? -> Resp length: ", string(len(pullRequests["values"].([]interface{}))))

	log.Println("--------- End PullRequestExists ---------")
	return len(pullRequests["values"].([]interface{})) > 0, nil
}

func (service *BitbucketService) CreatePullRequest(src string, dest string, repoName string, repoOwner string, reviewer string) error {

	log.Println("--------- START CreatePullRequest ---------")

	//NB!! Put back & test cos now sending in UUID
	exists, err := service.PullRequestExists(repoName, repoOwner, src, dest)

	log.Println("PullRequestExists -> err?: ", err)

	if err != nil {
		log.Fatal(err)
		return err
	}

	log.Println("PullRequestExists -> exists?: ", exists)
	if exists {
		log.Println("Skipping creation. Pull Request Exists: ", src, " -> ", dest)
		return nil
	}

	log.Println("Set options...")
	log.Println("repoOwner: ", repoOwner)
	log.Println("repoName: ", repoName)

	/* ORIGINAL
	options := bitbucket.PullRequestsOptions{
		ID:                 "",
		CommentID:          "",
		Owner:              repoOwner,
		RepoSlug:           repoName,
		Title:              "#AutomaticCascade " + src + " -> " + dest,
		Description:        "#AutomaticCascade " + src + " -> " + dest + ", this branch will automatically be merged on " +
			"successful build result+approval",
		CloseSourceBranch:  false,
		SourceBranch:       src,
		SourceRepository:   "",
		DestinationBranch:  dest,
		DestinationCommit:  "",
		Message:            "",
		//Reviewers:          []string{reviewer},
		//States:             nil,
		Query:              "",
		Sort:               "",
	}

	resp, err := service.bitbucketClient.Repositories.PullRequests.Create(&options)

	log.Println(service.PrettyPrint(resp))

	*/

	options := &bitbucket.PullRequestsOptions{
		Owner:             repoOwner,
		RepoSlug:          repoName,
		SourceBranch:      "release/appleufi_1.0",
		DestinationBranch: "feature/appleufi_1.0",
		Title:             "#AutomaticCascade " + src + " -> " + dest,
		Description: "#AutomaticCascade " + src + " -> " + dest + ", this branch will automatically be merged on " +
			"successful build result+approval",
		CloseSourceBranch: false,
	}
	//SourceBranch:      src,
	//DestinationBranch: dest,

	log.Println(service.PrettyPrint(options))

	log.Println("B4 CREATE pullRequests...")

	resp, err := service.bitbucketClient.Repositories.PullRequests.Create(options)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp)

	log.Println(service.PrettyPrint(resp))

	if err != nil {
		log.Fatal(err)
		return err
	}

	log.Println("--------- End CreatePullRequest ---------")
	return err
}

func (service *BitbucketService) DoApproveAndMerge(repoOwner string, repoName string) error {
	log.Println("--------- START DoApproveAndMerge ---------")

	log.Println("Set options...")
	log.Println("repoOwner: ", repoOwner)
	log.Println("repoName: ", repoName)

	options := bitbucket.PullRequestsOptions{
		Owner:    repoOwner,
		RepoSlug: repoName,
		Query:    "title ~ \"#AutomaticCascade\" AND state = \"OPEN\"",
		States:   []string{"OPEN"},
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
		log.Println("ID: ", prUnwrapped["id"])
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
	/* TODO: Uncomment to activate auto-merge!
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
