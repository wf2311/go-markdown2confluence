package confluence

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/naminomare/gogutil/fileio"
)

// https://docs.atlassian.com/atlassian-confluence/REST/6.5.2/#content/{id}/child/attachment

const (
	AttachmentNotFoundError = "attachment not found"
)

// Attachments ..
type Attachments struct {
	Results []Attachment `json:"results"`
	Size    int          `json:"size"`
}

// Attachment ...
type Attachment struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Title    string `json:"title"`
	Metadata struct {
		Comment   string `json:"comment"`
		MediaType string `json:"mediaType"`
	} `json:"metadata"`
	Version struct {
		Number int `json:"number"`
	} `json:"version"`
}

// AttachmentResults Results
type AttachmentResults struct {
	Results []AttachmentFetchResult `json:"results"`
	Start   float64                 `json:"start"`
	Limit   float64                 `json:"limit"`
	Size    float64                 `json:"size"`
	Links   map[string]string       `json:"_links"`
}

// AttachmentFetchResult ...
type AttachmentFetchResult struct {
	ID         string               `json:"id"`
	Type       string               `json:"type"`
	Status     string               `json:"status"`
	Title      string               `json:"title"`
	MetaData   AttachmentMetaData   `json:"metadata"`
	Extensions AttachmentExtensions `json:"extensions"`
	Expandable AttachmentExpandable `json:"_expandable"`
	Links      AttachmentLinks      `json:"_links"`
}

// AttachmentMetaData ...
type AttachmentMetaData struct {
	MediaType  string                 `json:"mediaType"`
	Labels     AttachmentLabels       `json:"labels"`
	Expandable map[string]interface{} `json:"_expandable"`
}

// AttachmentLabels ...
type AttachmentLabels struct {
	Results []interface{}     `json:"results"`
	Start   float64           `json:"start"`
	Limit   float64           `json:"limit"`
	Size    float64           `json:"size"`
	Links   map[string]string `json:"_links"`
}

// AttachmentExtensions Extensions
type AttachmentExtensions struct {
	MediaType string  `json:"mediaType"`
	FileSize  float64 `json:"fileSize"`
	Comment   string  `json:"comment"`
}

// AttachmentExpandable expandable
type AttachmentExpandable struct {
	Container    string `json:"container"`
	Operations   string `json:"operations"`
	Children     string `json:"children"`
	Restrictions string `json:"restrictions"`
	History      string `json:"history"`
	// Ancestors string `json:"ancestors"`
	// Body string `json:"body"`
	// Version string `json:"version"`
	Descendants string `json:"descendants"`
	Space       string `json:"space"`
}

// AttachmentLinks links
type AttachmentLinks struct {
	Self      string `json:"self"`
	Webui     string `json:"webui"`
	Download  string `json:"download"`
	Thumbnail string `json:"thumbnail"`
}

type UpdateAttachmentNameRequest struct {
	Title string `json:"title"`
	ID    string `json:"id"`
	Version Version `json:"version"`
}
type Version struct {
	Number    int  `json:"number"`
	MajorEdit bool `json:"majorEdit"`
}

// UnmarshalJSON Custom Unmarshaller
func (a *AttachmentLinks) UnmarshalJSON(data []byte) error {
	type Alias AttachmentLinks
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	err := json.Unmarshal(data, &aux)
	if err != nil {
		return err
	}

	a.Thumbnail = strings.Replace(a.Download, "attachments", "thumbnails", 1)

	// Dirty hack nees to convert image macro to use ! in storage mode
	a.Thumbnail = stripQueryParam(a.Thumbnail, "modificationDate")
	a.Thumbnail = stripQueryParam(a.Thumbnail, "cacheVersion")
	a.Thumbnail = stripQueryParam(a.Thumbnail, "api")
	a.Thumbnail = stripQueryParam(a.Thumbnail, "version")

	return nil
}

func stripQueryParam(inURL string, stripKey string) string {
	u, err := url.Parse(inURL)
	if err != nil {
		return inURL
	}
	q := u.Query()
	q.Del(stripKey)
	u.RawQuery = q.Encode()
	return u.String()
}

func (client *Client) newAttachmentEndpoint(contentID string) string {
	return "/rest/api/content/" + contentID + "/child/attachment"
}

func (client *Client) attachmentEndpoint(contentID, attachmentID string) string {
	return client.newAttachmentEndpoint(contentID) + "/" + attachmentID
}

func (client *Client) attachmentDataEndpoint(contentID, attachmentID string) string {
	return client.attachmentEndpoint(contentID, attachmentID) + "/data"
}

// DeleteAttachment ..
func (client *Client) DeleteAttachment(contentID string, attachmentID string) error {
	endpoint := client.attachmentEndpoint(contentID, attachmentID)

	_, err := client.request("DELETE", endpoint, "", nil)
	if err != nil {
		return err
	}

	return nil
}

// GetAttachment ...
func (client *Client) GetAttachment(contentID, attachmentID string) (*Attachment, error) {
	endpoint := client.attachmentEndpoint(contentID, attachmentID)

	res, err := client.request("GET", endpoint, "", nil)
	if err != nil {
		return nil, err
	}

	var attachments Attachments
	err = json.Unmarshal(res, &attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments.Results) < 1 {
		return nil, fmt.Errorf("empty list")
	}

	return &attachments.Results[0], nil
}

// GetAttachments ...
func (client *Client) GetAttachments(contentID string) (*[]Attachment, error) {
	endpoint := client.newAttachmentEndpoint(contentID)

	res, err := client.request("GET", endpoint, "", nil)
	if err != nil {
		return nil, err
	}

	var attachments Attachments
	err = json.Unmarshal(res, &attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments.Results) < 1 {
		return nil, fmt.Errorf("empty list")
	}
	return &attachments.Results, nil
}

// GetAttachmentByFilename ...
func (client *Client) GetAttachmentByFilename(contentID, filename string) (*Attachment, error) {
	endpoint := client.newAttachmentEndpoint(contentID)

	data := url.Values{}
	data.Set("filename", filename)
	query := data.Encode()

	res, err := client.request("GET", endpoint, query, nil)
	if err != nil {
		return nil, err
	}

	var attachments Attachments
	err = json.Unmarshal(res, &attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments.Results) < 1 {
		return nil, fmt.Errorf("attachment not found")
	}

	return &attachments.Results[0], nil
}

func (client *Client) UpdateAttachmentName(contentID, attachmentID string, path string) (*Attachment, error) {
	version := Version{
		Number:    1,
		MajorEdit: false,
	}
	request := UpdateAttachmentNameRequest{
		ID:    attachmentID,
		Title: path,
		Version: version,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	endpoint := client.attachmentEndpoint(contentID, attachmentID)
	res, err := client.request("PUT", endpoint, "", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	var attachment Attachment
	err = json.Unmarshal(res, &attachment)
	if err != nil {
		return nil, err
	}
	return &attachment, nil
}

// UpdateAttachment ...
func (client *Client) UpdateAttachment(contentID, attachmentID, path string, minorEdit bool) (*Attachment, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}


	md5HashString, err := GetFileMD5Hash(path)
	if err != nil {
		return nil, err
	}

	part, err := writer.CreateFormFile("file", md5HashString+"_"+fi.Name())
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}

	err = writer.WriteField("minorEdit", strconv.FormatBool(minorEdit))
	if err != nil {
		return nil, err
	}

	err = writer.WriteField("comment", md5HashString)
	if err != nil {
		return nil, err
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	endpoint := client.attachmentDataEndpoint(contentID, attachmentID)
	if err != nil {
		return nil, err
	}

	preRequest := func(req *http.Request) {
		req.Header.Set("Content-Type", writer.FormDataContentType())
	}

	res, err := client.request("POST", endpoint, "", body, preRequest)
	if err != nil {
		return nil, err
	}

	var attachment Attachment
	err = json.Unmarshal(res, &attachment)
	if err != nil {
		return nil, err
	}
	return &attachment, nil
}

// AddAttachment ...
func (client *Client) AddAttachment(contentID, path string) (*Attachment, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	md5HashString, err := GetFileMD5Hash(path)
	if err != nil {
		return nil, err
	}

	part, err := writer.CreateFormFile("file", md5HashString+"_"+fi.Name())
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}

	err = writer.WriteField("comment", md5HashString)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}
	endpoint := client.newAttachmentEndpoint(contentID)
	if err != nil {
		return nil, err
	}
	preRequest := func(req *http.Request) {
		req.Header.Set("Content-Type", writer.FormDataContentType())
	}

	res, err := client.request("POST", endpoint, "", body, preRequest)
	if err != nil {
		return nil, err
	}

	var attachments Attachments
	err = json.Unmarshal(res, &attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments.Results) < 1 {
		return nil, fmt.Errorf("empty list")
	}

	return &attachments.Results[0], nil
}

// AddUpdateAttachments ...
func (client *Client) AddUpdateAttachments(contentID string, files []string) ([]*Attachment, []error) {
	var results []*Attachment
	var errors []error

	attachmentsMap, _ := client.GetPageAttachmentsAndToMap(contentID)

	for _, f := range files {
		filename := path.Base(f)
		attachment, err := matchAttachmentByMd5(f, attachmentsMap)
		if err != nil || attachment == nil {
			attachment, err = client.AddAttachment(contentID, f)
		} else {
			fmt.Println(fmt.Sprintf("attachment %s already exists, skipping,md5=%s", filename,
				attachment.Metadata.Comment))
			filename_with_md5 := attachment.Metadata.Comment + "_" + filename
			if filename_with_md5 != attachment.Title {
				fmt.Println(fmt.Sprintf("updating attachment %s name to %s", attachment.Title, filename_with_md5))
				attachment, err = client.UpdateAttachmentName(contentID, attachment.ID, filename_with_md5)
			}
		}
		if err == nil {
			results = append(results, attachment)
		} else {
			errors = append(errors, err)
		}
	}
	return results, errors
}

func matchAttachmentByMd5(path string, maps map[string]*Attachment) (*Attachment, error) {
	md5HashString, err := GetFileMD5Hash(path)
	if err != nil {
		return nil, err
	}
	attachment := maps[md5HashString]
	//if attachment != nil {
	//	fmt.Println(fmt.Sprintf("match attachment %s by md5=%s", attachment.Title, md5HashString))
	//}
	return attachment, nil
}

func (client *Client) GetPageAttachmentsAndToMap(pageID string) (map[string]*Attachment, error) {
	attachments, err := client.GetAttachments(pageID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*Attachment)
	for i, a := range *attachments {
		m[a.Metadata.Comment] = &(*attachments)[i]
		//fmt.Println(fmt.Sprintf("page:%s,exist attachment %s,md5=%s", pageID, a.Title, a.Metadata.Comment))
	}
	return m, nil
}

// FetchAttachmentMetaData ...
func (client *Client) FetchAttachmentMetaData(contentID string) (*AttachmentResults, error) {
	endpoint := client.newAttachmentEndpoint(contentID)

	res, err := client.request(
		http.MethodGet,
		endpoint,
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}

	var attachments AttachmentResults
	err = json.Unmarshal(res, &attachments)
	if err != nil {
		return nil, err
	}
	if len(attachments.Results) < 1 {
		return nil, fmt.Errorf("empty list")
	}

	return &attachments, err
}

// DownloadAttachmentsFromPage ...
func (client *Client) DownloadAttachmentsFromPage(pageID, directory string) error {
	res, err := client.FetchAttachmentMetaData(pageID)
	if err != nil {
		return err
	}

	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return err
	}

	for _, v := range res.Results {
		downloadURL := client.Endpoint + v.Links.Download
		path, err := fileio.GetNonExistFileName(filepath.Join(directory, v.Title), 1000)
		if err != nil {
			return err
		}
		err = client.DownloadFromURL(downloadURL, path)
		if err != nil {
			return err
		}
	}
	return nil
}

// DownloadFromURL ...
func (client *Client) DownloadFromURL(url, outputFilepath string) error {
	resp, err := client.request(
		http.MethodGet,
		url,
		"",
		nil,
	)
	if err != nil {
		return err
	}
	fh, err := os.Create(outputFilepath)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(resp)

	return err
}


func GetFileMD5Hash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

