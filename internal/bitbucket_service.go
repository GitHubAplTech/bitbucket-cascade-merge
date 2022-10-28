package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ktrysmt/go-bitbucket"
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

/*** Utility Functions ***/
/* ===================== */

// PrettyPrint to print struct in a readable way
func (service *BitbucketService) PrettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

// GetStringInBetween Returns empty string if no start string found
func (service *BitbucketService) GetStringInBetween(value string, a string, b string) string {
	// Get substring between two strings.
	posFirst := strings.Index(value, a)
	if posFirst == -1 {
		return ""
	}
	posLast := strings.Index(value, b)
	if posLast == -1 {
		return ""
	}
	posFirstAdjusted := posFirst + len(a)
	if posFirstAdjusted >= posLast {
		return ""
	}
	return value[posFirstAdjusted:posLast]
}

/*** EXISTING PR -> AUTO APPROVE & MERGE ***/
/* ======================================= */

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

func (service *BitbucketService) DoApproveAndMerge(repoOwner string, repoName string) error {
	log.Println("--------- START DoApproveAndMerge ---------")

	log.Println("Set options...")
	log.Println("repoOwner: ", repoOwner)
	log.Println("repoName: ", repoName)

	options := bitbucket.PullRequestsOptions{
		Owner:    repoOwner,
		RepoSlug: repoName,
		Query:    "state = \"OPEN\"",
		States:   []string{"OPEN"},
	}
	//Only auto approve & merge when includes #AutoCascade
	//Query:    "title ~ \"#AutoCascade\" AND state = \"OPEN\"",

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
		log.Println("Destination: ", prUnwrapped["destination"].(map[string]interface{})["branch"].(map[string]interface{})["name"])
		log.Println("Trying to Auto Approve...")

		err = service.ApprovePullRequest(repoOwner, repoName, fmt.Sprintf("%v", prUnwrapped["id"]), fmt.Sprintf("%v", prUnwrapped["destination"].(map[string]interface{})["branch"].(map[string]interface{})["name"]))
		if err != nil {
			return err
		}
	}

	log.Println("--------- End DoApproveAndMerge ---------")

	return nil
}

// HACK: There isn't an API method in the Bitbucket API Library to do pull request
// approval. Hacking together one here.
func (service *BitbucketService) ApprovePullRequest(repoOwner string, repoName string, pullRequestId string, destBranch string) error {
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
	log.Println(service.PrettyPrint(buf))

	//Try merge (if not UAT or Release)
	if !strings.HasPrefix(destBranch, "uat") && !strings.HasPrefix(destBranch, service.ReleaseBranchPrefix) {
		log.Println("Try to Auto Merge -> ", destBranch)
		err = service.MergePullRequest(repoOwner, repoName, pullRequestId)
		if err != nil {
			return err
		}
	} else {
		log.Println("SKIP Auto Merge -> ", destBranch)
	}

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
	resp, err := service.bitbucketClient.Repositories.PullRequests.Merge(&options)
	log.Println(service.PrettyPrint(resp))
	if err != nil {
		log.Println("error: ", err)
		/* Don't return error (causes crash)
		return err */
	}

	log.Println("--------- End MergePullRequest ---------")
	return nil
}

/*** AFTER MERGE -> CREATE NEXT BRANCH PR ***/
/* ======================================== */

func (service *BitbucketService) OnMerge(request *PullRequestMergedPayload) error {
	log.Println("--------- START OnMerge ---------")

	// Only operate on release branches
	sourceBranchName := request.PullRequest.Source.Branch.Name
	destBranchName := request.PullRequest.Destination.Branch.Name
	authorId := request.PullRequest.Author.UUID

	log.Println("sourceBranchName", sourceBranchName)
	log.Println("destBranchName", destBranchName)
	log.Println("authorId", authorId)

	origTitle := request.PullRequest.Title
	log.Println("Orig origTitle", origTitle)
	siteSpecific := (destBranchName != service.DevelopmentBranchName && !strings.HasPrefix(origTitle, "#AutoCascade "))

	origTitle = strings.ReplaceAll(origTitle, "#AutoCascade ", "")
	log.Println("Replaced origTitle", origTitle)

	// NB!!! UNCOMMENT if only want create on 1st merge!
	//if strings.HasPrefix(destBranchName, service.ReleaseBranchPrefix) {
	//log.Println("Inside blk -> Only operate on release branches")

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
	log.Println("siteSpecific: ", siteSpecific)

	//targets, err := service.GetBranches(repoName, repoOwner)
	targets, err := service.GetBranches(repoName, request.Repository.Owner.UUID)

	if err != nil {
		return err
	}
	log.Println("Checking for internal targets: ", targets)

	//Cater for starting in dev branch of particular site
	if siteSpecific {
		log.Println("Site-specific commit!")

		nextTarget := service.SiteSpecificNextTarget(destBranchName, targets)

		if nextTarget != "" {
			log.Println("Call Create PR (Site-specific) -> Next Target: ", string(nextTarget))
			//err = service.CreatePullRequest(destBranchName, nextTarget, repoName, repoOwner, authorId)
			err = service.CreatePullRequest(origTitle, destBranchName, nextTarget, repoName, request.Repository.Owner.UUID, authorId)
			if err != nil {
				log.Println("err: ", err)
				//return err
			}
		} else {
			log.Println("SKIP Create PR (Site-specific) -> Next Target: ", string(nextTarget))
		}

		//Propagate to all site dev branches
	} else {
		log.Println("All-sites commit!")

		err := service.AllSitesNextTarget(destBranchName, targets, origTitle, repoName, request.Repository.Owner.UUID, authorId)

		if err != nil {
			log.Println("err: ", err)
			//return err
		}
	}
	//}

	log.Println("--------- End OnMerge ---------")
	return nil
}

// Site-Specific merge path
func (service *BitbucketService) SiteSpecificNextTarget(oldDest string, cascadeTargets *[]string) string {
	targets := *cascadeTargets

	log.Println("--------- START SiteSpecificNextTarget ---------")

	//Loop to find next target based on destination of merged PR
	for i, target := range targets {
		log.Println("Target Loop: ", i)
		log.Println("oldDest: ", target)
		log.Println("target: ", target)

		//Main to Dev
		if oldDest == service.DevelopmentBranchName && strings.HasPrefix(target, "dev") {
			//check same site name
			if service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
				log.Println("Dev to QA: ", target)
				return target
			}
		}
		//Dev to QA
		if strings.HasPrefix(oldDest, "dev") && strings.HasPrefix(target, "qa") {
			//check same site name
			if service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
				log.Println("Dev to QA: ", target)
				return target
			}
		}
		//QA to UAT
		if strings.HasPrefix(oldDest, "qa") && strings.HasPrefix(target, "uat") {
			//check same site name
			if service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
				log.Println("QA to UAT: ", target)
				return target
			}
		}
		//UAT to Live
		if strings.HasPrefix(oldDest, "uat") && strings.HasPrefix(target, service.ReleaseBranchPrefix) {
			//check same site name
			if service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
				log.Println("QA to UAT: ", target)
				return target
			}
		}
	}

	log.Println("--------- End SiteSpecificNextTarget ---------")
	//Fallback on no desitination branch
	return ""
}

// Mine
func (service *BitbucketService) AllSitesNextTarget(oldDest string, cascadeTargets *[]string, origTitle string, repoName string, repoOwner string, authorId string) error {
	targets := *cascadeTargets

	log.Println("--------- START AllSitesNextTarget ---------")

	//Loop to find next target based on destination of merged PR
	for i, target := range targets {
		log.Println("Target Loop: ", i)
		log.Println("oldDest: ", oldDest)
		log.Println("target: ", target)
		log.Println("oldDest Site: ", service.GetStringInBetween(oldDest, "/", "_"))
		log.Println("target Site: ", service.GetStringInBetween(target, "/", "_"))

		//Main to Dev
		if oldDest == service.DevelopmentBranchName && strings.HasPrefix(target, "dev") {
			log.Println("Dev to QA: Call Create PR (All-sites) -> Next Target: ", target)
			err := service.CreatePullRequest(origTitle, oldDest, target, repoName, repoOwner, authorId)
			if err != nil {
				log.Println("err: ", err)
				//return err
			}
		}
		//Dev to QA
		if strings.HasPrefix(oldDest, "dev") && strings.HasPrefix(target, "qa") &&
			//check same site name
			service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
			log.Println("Dev to QA: Call Create PR (All-sites) -> Next Target: ", target)
			err := service.CreatePullRequest(origTitle, oldDest, target, repoName, repoOwner, authorId)
			if err != nil {
				log.Println("err: ", err)
				//return err
			}
		}
		//QA to UAT
		if strings.HasPrefix(oldDest, "qa") && strings.HasPrefix(target, "uat") &&
			//check same site name
			service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
			log.Println("QA to UAT: Call Create PR (All-sites) -> Next Target: ", target)
			err := service.CreatePullRequest(origTitle, oldDest, target, repoName, repoOwner, authorId)
			if err != nil {
				log.Println("err: ", err)
				//return err
			}
		}
		//UAT to Release
		if strings.HasPrefix(oldDest, "uat") && strings.HasPrefix(target, service.ReleaseBranchPrefix) &&
			//check same site name
			service.GetStringInBetween(oldDest, "/", "_") == service.GetStringInBetween(target, "/", "_") {
			log.Println("UAT to Release: Call Create PR (All-sites) -> Next Target: ", target)
			err := service.CreatePullRequest(origTitle, oldDest, target, repoName, repoOwner, authorId)
			if err != nil {
				log.Println("err: ", err)
				//return err
			}
		}
	}

	log.Println("--------- End AllSitesNextTarget ---------")
	//Fallback on no desitination branch
	return nil
}

/* ORIGINAL (with versioning included)
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
*/

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
		Query:    "state = \"OPEN\" AND destination.branch.name = \"" + destination + "\" AND source.branch.name=\"" + source + "\"",
		States:   []string{"OPEN"},
	}

	log.Println("B4 GET pullRequests...")

	resp, err := service.bitbucketClient.Repositories.PullRequests.Gets(&options)
	if err != nil {
		log.Println(err)
		return false, nil
	}

	pullRequests := resp.(map[string]interface{})
	log.Println("Pull Req exists? -> Resp length: ", string(len(pullRequests["values"].([]interface{}))))

	log.Println("--------- End PullRequestExists ---------")
	return len(pullRequests["values"].([]interface{})) > 0, nil
}

func (service *BitbucketService) CreatePullRequest(origTitle string, src string, dest string, repoName string, repoOwner string, reviewer string) error {

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

	options := &bitbucket.PullRequestsOptions{
		Owner:             repoOwner,
		RepoSlug:          repoName,
		SourceBranch:      src,
		DestinationBranch: dest,
		Title:             "#AutoCascade " + origTitle,
		Description: "#AutoCascade " + src + " -> " + dest + ", this branch will automatically be merged on " +
			"successful build result+approval",
		CloseSourceBranch: false,
	}
	//SourceBranch:      "release/appleufi_1.0",
	//DestinationBranch: "feature/appleufi_1.0",

	log.Println(service.PrettyPrint(options))

	log.Println("B4 CREATE pullRequests...")

	resp, err := service.bitbucketClient.Repositories.PullRequests.Create(options)
	if err != nil {
		log.Println(service.PrettyPrint(err))
		//panic(err)
	}

	log.Println(service.PrettyPrint(resp))

	log.Println("--------- End CreatePullRequest ---------")
	return err
}
