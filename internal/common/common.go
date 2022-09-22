package common

import (
	"errors"
	"fmt"
	"net/http"
)

func GetWithRetry(req *http.Request, name string) (*http.Response, error) {
	var resp *http.Response
	var err error

	validResp, retries := false, 3
	for !validResp {
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			if retries > 1 {
				retries--
				continue
			} else {
				err = errors.New(fmt.Sprintf("error on %v api request: %s", name, err.Error()))
				return nil, err
			}
		} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
			resp.Body.Close()
			if retries > 1 {
				retries--
				continue
			} else {
				err = errors.New(fmt.Sprintf("error code %v returned from %v", name, resp.StatusCode))
				return nil, err
			}
		} else {
			validResp = true
		}
	}
	return resp, nil
}
