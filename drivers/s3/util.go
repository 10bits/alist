package s3

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

func (d *S3) initSession() error {
	cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(d.AccessKeyID, d.SecretAccessKey, ""),
		Region:           &d.Region,
		Endpoint:         &d.Endpoint,
		S3ForcePathStyle: aws.Bool(d.ForcePathStyle),
	}
	var err error
	d.Session, err = session.NewSession(cfg)
	return err
}

func (d *S3) getClient(link bool) *s3.S3 {
	client := s3.New(d.Session)
	if link && d.CustomHost != "" {
		client.Handlers.Build.PushBack(func(r *request.Request) {
			if r.HTTPRequest.Method != http.MethodGet {
				return
			}
			r.HTTPRequest.URL.Host = d.CustomHost
		})
	}
	return client
}

func getKey(path string, dir bool) string {
	path = strings.TrimPrefix(path, "/")
	if path != "" && dir {
		path += "/"
	}
	return path
}

var defaultPlaceholderName = ".placeholder"

func getPlaceholderName(placeholder string) string {
	if placeholder == "" {
		return defaultPlaceholderName
	}
	return placeholder
}

func (d *S3) listV1(prefix string) ([]model.Obj, error) {
	prefix = getKey(prefix, true)
	log.Debugf("list: %s", prefix)
	files := make([]model.Obj, 0)
	marker := ""
	for {
		input := &s3.ListObjectsInput{
			Bucket:    &d.Bucket,
			Marker:    &marker,
			Prefix:    &prefix,
			Delimiter: aws.String("/"),
		}
		listObjectsResult, err := d.client.ListObjects(input)
		if err != nil {
			return nil, err
		}
		for _, object := range listObjectsResult.CommonPrefixes {
			name := path.Base(strings.Trim(*object.Prefix, "/"))
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Modified: d.Modified,
				IsFolder: true,
			}
			files = append(files, &file)
		}
		for _, object := range listObjectsResult.Contents {
			name := path.Base(*object.Key)
			if name == getPlaceholderName(d.Placeholder) {
				continue
			}
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Size:     *object.Size,
				Modified: *object.LastModified,
			}
			files = append(files, &file)
		}
		if listObjectsResult.IsTruncated == nil {
			return nil, errors.New("IsTruncated nil")
		}
		if *listObjectsResult.IsTruncated {
			marker = *listObjectsResult.NextMarker
		} else {
			break
		}
	}
	return files, nil
}

func (d *S3) listV2(prefix string) ([]model.Obj, error) {
	prefix = getKey(prefix, true)
	files := make([]model.Obj, 0)
	var continuationToken, startAfter *string
	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            &d.Bucket,
			ContinuationToken: continuationToken,
			Prefix:            &prefix,
			Delimiter:         aws.String("/"),
			StartAfter:        startAfter,
		}
		listObjectsResult, err := d.client.ListObjectsV2(input)
		if err != nil {
			return nil, err
		}
		log.Debugf("resp: %+v", listObjectsResult)
		for _, object := range listObjectsResult.CommonPrefixes {
			name := path.Base(strings.Trim(*object.Prefix, "/"))
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Modified: d.Modified,
				IsFolder: true,
			}
			files = append(files, &file)
		}
		for _, object := range listObjectsResult.Contents {
			name := path.Base(*object.Key)
			if name == getPlaceholderName(d.Placeholder) {
				continue
			}
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Size:     *object.Size,
				Modified: *object.LastModified,
			}
			files = append(files, &file)
		}
		if !aws.BoolValue(listObjectsResult.IsTruncated) {
			break
		}
		if listObjectsResult.NextContinuationToken != nil {
			continuationToken = listObjectsResult.NextContinuationToken
			continue
		}
		if len(listObjectsResult.Contents) == 0 {
			break
		}
		startAfter = listObjectsResult.Contents[len(listObjectsResult.Contents)-1].Key
	}
	return files, nil
}

func (d *S3) copy(src string, dst string, isDir bool) error {
	srcKey := getKey(src, isDir)
	dstKey := getKey(dst, isDir)
	input := &s3.CopyObjectInput{
		Bucket:     &d.Bucket,
		CopySource: &srcKey,
		Key:        &dstKey,
	}
	_, err := d.client.CopyObject(input)
	return err
}
