package internal

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type BitbucketController struct {
	bitbucketService   *BitbucketService
	BitbucketSharedKey string
}

const PrFufilled = "pullrequest:fulfilled"

// NB!!! Revert to above again after testing!
// const PrFufilled = "pullrequest:approved"

const PrCommentTrigger = "pullrequest:comment_created"

func NewBitbucketController(bitbucketService *BitbucketService, bitbucketSharedKey string) *BitbucketController {
	return &BitbucketController{bitbucketService, bitbucketSharedKey}
}

func (ctrl *BitbucketController) Webhook(c *gin.Context) {

	var PullRequestPayload PullRequestMergedPayload

	buf, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(buf, &PullRequestPayload)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("c.Request.Header.Get(X-Event-Key): ", c.Request.Header.Get("X-Event-Key"))

	if ctrl.validate(c.Request) {
		go func() {
			var err error
			var PrForceRetrigger bool

			// Detect a force-retrigger
			if c.Request.Header.Get("X-Event-Key") == PrCommentTrigger {
				log.Println("In Detect a force-retrigger. Comment=", PullRequestPayload.Comment.Content.Raw)

				// Only counts if comment = "#AutoCascade or new Jira editor is `#AutoCascade`"
				if strings.TrimSpace(PullRequestPayload.Comment.Content.Raw) == "#AutoCascade" || strings.TrimSpace(PullRequestPayload.Comment.Content.Raw) == "`#AutoCascade`" {
					log.Println("In Set PrForceRetrigger = true")
					PrForceRetrigger = true
				}

			}

			// Fork for logic processing
			if c.Request.Header.Get("X-Event-Key") == PrFufilled || PrForceRetrigger {
				err = ctrl.bitbucketService.OnMerge(&PullRequestPayload)
			} else {
				err = ctrl.bitbucketService.TryMerge(&PullRequestPayload)
			}
			if err != nil {
				log.Fatal(err)
			}
		}()

		c.JSON(http.StatusOK, nil)
	} else {
		c.JSON(http.StatusForbidden, nil)
	}
}

func (ctrl *BitbucketController) validate(request *http.Request) bool {
	keys, ok := request.URL.Query()["key"]
	if !ok || len(keys[0]) < 1 {
		log.Println("Url Param 'key' is missing")
	}
	key := keys[0]
	return ctrl.BitbucketSharedKey == key
}
