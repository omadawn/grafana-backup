package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
)

var (
	testURL = "http://noserver.nodomain.com:3123"
)

func TestRestoreDashboards(t *testing.T) {
	t.Log("TestRestoreDashboards not yet implemented!")
}

//TODO: Create multiple tests which test things like sending multiple files
//TODO: Create something that uses a 200 response. See do-backup_test for better examples.
func TestRestoreDatasources(t *testing.T) {

	*flagServerURL, _ = url.Parse(testURL)
	*flagServerKey = "thisisnotreallyanapikey"
	*flagApplyFor = "datasources"
	*restorePath = "testdata/prometheus-test.ds.1.json"

	// For developing tests. Both of these cause this test to fail.
	//*restorePath = "testdata/*.1.json"
	//*restorePath = "testdata/promartheus-test.ds.1.json"

	argPath      = restorePath

	// Some variables to track the results of the test

	// Check the accept header.
	acceptCorrect    := false
	// Check for some expected text in the post body.
	bodyCorrect      := false
	// Track how many times the API was called.
	numRequests      := 0
	// Were any requests made to other URIs?
	wrongUriRequests := false

	// Set up httpmock
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	//TODO: Break this up into multiple functions so that the NoResponder doesn't cause us to fail Accept Header, body, etc.
	// Create a responder which will respond with valid JSON and check what was posted to us for validity.
	httpmock.RegisterResponder("POST", testURL + "/api/datasources",
		func(req *http.Request) (*http.Response, error) {

			numRequests++

			if strings.Contains(req.Header.Get("Accept"), "application/json") {
				acceptCorrect = true
			}

			//TODO: Expand this to unmarshal the JSON and check specific fields for specific values.

			// Get a string out of the io.ReadCloser
			buf := new(bytes.Buffer)
			buf.ReadFrom(req.Body)
			postBody := buf.String() // Does a complete copy of the bytes in the buffer.

			if strings.Contains(postBody, "prometheus-test") {
				bodyCorrect = true
			}

			// Uncomment for troubleshooting.
			//fmt.Printf("Request headers: \n%v\n", req.Header)
			//
			//fmt.Printf("Request body: \n%s\n", postBody)

			return httpmock.NewStringResponse(409, `{ "message": "This response is from the mocking framework!" }`), nil

			//TODO: Figure out how to make sure that do-restore is throwing an error when we return anything other than "Datasource added"
		},
	)

	httpmock.RegisterNoResponder(
		func(req *http.Request) (*http.Response, error) {

			wrongUriRequests = true

			fmt.Printf("Unexpected Request: \n%v\n", req)

			//fmt.Printf("Request headers: \n%v\n", req.Header)
			//
			//fmt.Printf("Request body: \n%v\n", req.Body)

			return httpmock.NewStringResponse(500, `{ "message": "Unexpected request" }`), nil
		},
	)

	doRestore(serverInstance, applyFor, matchFilename)

	if acceptCorrect != true {
		t.Error("Accept header was invalid.")
		//t.Fail()
	}

	if bodyCorrect != true {
		t.Error("Expected text not found in the POST body.")
		//t.Fail()
	}

	if numRequests != 1 {
		t.Errorf("The /api/datasources URI was called an incorrect number of times. Actual requests %d", numRequests)
	}

	if wrongUriRequests != false {
		t.Error("Request made to an unexpected URI. See the log for details.")
	}
}

//TODO: Change t.Log to t.Error when ready to implement this.
func TestRestoreUsers(t *testing.T) {
	t.Log("Test Restore Users not yet implemented because restoring users is not yet implemented.")
}
