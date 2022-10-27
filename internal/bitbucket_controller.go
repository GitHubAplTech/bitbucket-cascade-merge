package internal

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type BitbucketController struct {
	bitbucketService   *BitbucketService
	BitbucketSharedKey string
}

const PrFufilled = "pullrequest:fulfilled"
const PrTestRetrigger = "pullrequest:comment_created"

// NB!!! Revert to above again after testing!
//const PrFufilled = "pullrequest:approved"

func NewBitbucketController(bitbucketService *BitbucketService, bitbucketSharedKey string) *BitbucketController {
	return &BitbucketController{bitbucketService, bitbucketSharedKey}
}

func (ctrl *BitbucketController) Webhook(c *gin.Context) {

	var PullRequestMerged PullRequestMergedPayload

	buf, err := ioutil.ReadAll(c.Request.Body)
	err = json.Unmarshal(buf, &PullRequestMerged)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("c.Request.Header.Get(X-Event-Key): ", c.Request.Header.Get("X-Event-Key"))

	if ctrl.validate(c.Request) {
		go func() {
			var err error
			if c.Request.Header.Get("X-Event-Key") == PrFufilled || c.Request.Header.Get("X-Event-Key") == PrTestRetrigger {
				err = ctrl.bitbucketService.OnMerge(&PullRequestMerged)
			} else {
				err = ctrl.bitbucketService.TryMerge(&PullRequestMerged)
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
	if ctrl.BitbucketSharedKey == key {
		return true
	}
	return false
}
