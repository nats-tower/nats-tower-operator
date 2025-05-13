package natstower

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

type NATSTowerClientConfig struct {
	ClusterID       string
	NATSTowerURL    string
	NATSTowerAPIKey string
}

// NATSTowerClient ...
type NATSTowerClient struct {
	ctx        context.Context
	cfg        NATSTowerClientConfig
	httpClient *http.Client
}

// CreateNATSTowerClient ...
func CreateNATSTowerClient(ctx context.Context,
	cfg NATSTowerClientConfig) (*NATSTowerClient, error) {
	t := &NATSTowerClient{
		ctx: ctx,
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	return t, nil
}

type ConnectionInfo struct {
	Creds       string
	URLs        string
	AccountName string
}

type listResponse[T listItems] struct {
	Items []T `json:"items"`
}

type listItems interface {
	operator | account | user | limits | k8sAccess
}

type operator struct {
	URLs string `json:"url"`
	ID   string `json:"id"`
}

type limits struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

type user struct {
	ID    string `json:"id"`
	Creds string `json:"creds"`
}

type k8sAccess struct {
}

func (c *NATSTowerClient) doJSONRequest(ctx context.Context,
	req *http.Request,
	v interface{}) error {
	req.Header.Set("X-Token", c.cfg.NATSTowerAPIKey)

	resp, err := c.httpClient.Do(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode > 299 {
		klog.Infof("%s resp: %s", req.URL.String(), string(b))
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if v != nil {
		err = json.Unmarshal(b, v)
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	ErrOperatorNotFound    = fmt.Errorf("operator not found")
	ErrAccountNotFound     = fmt.Errorf("account not found")
	ErrAccountTierNotFound = fmt.Errorf("account tier not found")
	ErrUserNotFound        = fmt.Errorf("user not found")
	ErrK8sAccessNotAllowed = fmt.Errorf("k8s access not allowed")
)

func (c *NATSTowerClient) getOperator(ctx context.Context,
	installationPublicKey string) (*operator, error) {

	req, err := http.NewRequestWithContext(ctx, "GET", c.cfg.NATSTowerURL+"/api/collections/nats_auth_operators/records", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()

	queryFilter := "public_key = '" + installationPublicKey + "'"
	q.Add("filter", queryFilter)
	q.Add("perPage", "1")
	q.Add("fields", "id,url")
	req.URL.RawQuery = q.Encode()

	var resp listResponse[operator]

	err = c.doJSONRequest(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, ErrOperatorNotFound
	}

	return &resp.Items[0], nil
}

func (c *NATSTowerClient) getAccount(ctx context.Context,
	operatorID, accountName string) (*account, error) {

	req, err := http.NewRequestWithContext(ctx, "GET", c.cfg.NATSTowerURL+"/api/collections/nats_auth_accounts/records", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("(operator='%s' && name='%s')", operatorID, accountName))
	q.Add("perPage", "1")
	q.Add("fields", "id,name,public_key")
	req.URL.RawQuery = q.Encode()

	var resp listResponse[account]

	err = c.doJSONRequest(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, ErrAccountNotFound
	}

	return &resp.Items[0], nil
}

func (c *NATSTowerClient) getUser(ctx context.Context,
	accountID, username string) (*user, error) {

	req, err := http.NewRequestWithContext(ctx, "GET", c.cfg.NATSTowerURL+"/api/collections/nats_auth_users/records", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("(account='%s' && name='%s')", accountID, username))
	q.Add("perPage", "1")
	q.Add("fields", "creds,id")
	req.URL.RawQuery = q.Encode()

	var resp listResponse[user]

	err = c.doJSONRequest(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, ErrUserNotFound
	}

	return &resp.Items[0], nil
}

func (c *NATSTowerClient) createUser(ctx context.Context,
	accountID, username, description string) (*user, error) {

	req, err := http.NewRequestWithContext(ctx,
		"POST",
		c.cfg.NATSTowerURL+"/api/collections/nats_auth_users/records",
		bytes.NewBuffer([]byte(fmt.Sprintf(`{"account":"%s","name":"%s","description":"%s"}`,
			accountID, username, description))))
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("fields", "creds,id")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")

	var resp user

	err = c.doJSONRequest(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *NATSTowerClient) CreateOrGetUserAuth(ctx context.Context,
	namespace,
	installationPublicKey string,
	accountName,
	name,
	description string) (*ConnectionInfo, error) {

	operator, err := c.getOperator(ctx, installationPublicKey)
	if err != nil {
		return nil, err
	}

	account, err := c.getAccount(ctx, operator.ID, accountName)
	if err != nil {
		return nil, err
	}

	allowed, err := c.accessAllowed(ctx, c.cfg.ClusterID, namespace, account.ID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrK8sAccessNotAllowed
	}

	user, err := c.getUser(ctx, account.ID, name)
	if err != nil && err != ErrUserNotFound {
		return nil, err
	}
	if ErrUserNotFound == err {
		// Create new user
		user, err = c.createUser(ctx, account.ID, name, description)
		if err != nil {
			return nil, err
		}
	}

	return &ConnectionInfo{
		Creds:       user.Creds,
		URLs:        operator.URLs,
		AccountName: account.Name,
	}, nil
}

func (c *NATSTowerClient) RemoveUserAuth(ctx context.Context,
	namespace,
	installationPublicKey,
	accountName,
	name string) error {

	operator, err := c.getOperator(ctx, installationPublicKey)
	if err != nil {
		if err == ErrOperatorNotFound {
			return nil
		}
		return err
	}

	account, err := c.getAccount(ctx, operator.ID, accountName)
	if ErrAccountNotFound == err {
		return nil
	}
	if err != nil {
		return err
	}

	allowed, err := c.accessAllowed(ctx, c.cfg.ClusterID, namespace, account.ID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrK8sAccessNotAllowed
	}

	user, err := c.getUser(ctx, account.ID, name)
	if ErrUserNotFound == err {
		return nil
	}
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx,
		"DELETE",
		c.cfg.NATSTowerURL+"/api/collections/nats_auth_users/records/"+user.ID,
		nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Token", c.cfg.NATSTowerAPIKey)

	resp, err := c.httpClient.Do(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *NATSTowerClient) accessAllowed(ctx context.Context, clusterID, namespace, accountID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx,
		"GET",
		c.cfg.NATSTowerURL+"/api/collections/nats_auth_k8s_access/records",
		nil)
	if err != nil {
		return false, err
	}
	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("(cluster='%s' && namespace='%s' && account='%s')", clusterID, namespace, accountID))
	q.Add("perPage", "1")
	req.URL.RawQuery = q.Encode()

	var resp listResponse[k8sAccess]

	err = c.doJSONRequest(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	if len(resp.Items) == 0 {
		return false, nil
	}

	return true, nil

}
