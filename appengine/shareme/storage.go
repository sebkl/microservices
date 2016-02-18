package shareme

import (
	"appengine"
	"appengine/blobstore"
	. "github.com/sebkl/gotojs"
	"github.com/sebkl/gotojs/gae"
	"github.com/sebkl/imgurl"
	"io"
	"io/ioutil"
	"log"
	"net/url"
)

const (
	BlobstoreStorageType = iota
)

type BlobstoreStorageService struct {
	UploadURLPrefix string
}

func (st *BlobstoreStorageService) Thumbnail(c *gae.Context, key string, w, h int) (ret string) {
	stat, err := st.Stat(c, key)
	if err != nil {
		log.Printf("Blobstore.Thumbnail '%s': %s", key, err)
		return
	}

	reader, err := st.Get(c, key)
	if err != nil {
		log.Printf("Blobstore.Thumbnail '%s': %s", key, err)
		return
	}

	ret, _, err = imgurl.UrlifyR(reader, stat.MimeType, w, h)
	return
}

func (st *BlobstoreStorageService) Get(c *gae.Context, key string) (ret io.ReadCloser, err error) {
	k := appengine.BlobKey(key)
	_, err = blobstore.Stat(c, k)
	if err == nil {
		ret = ioutil.NopCloser(blobstore.NewReader(c, k))
	}
	return
}

func (st *BlobstoreStorageService) Stat(c *gae.Context, key string) (*Share, error) {
	bi, err := blobstore.Stat(c, appengine.BlobKey(key))
	if err != nil {
		return nil, err
	}
	stat := newShare(bi)
	return stat, err
}

func (st *BlobstoreStorageService) Delete(c *gae.Context, key string) (err error) {
	_, err = st.Stat(c, key)
	if err != nil {
		return
	}

	err = blobstore.Delete(c, appengine.BlobKey(key))
	return
}

func (st *BlobstoreStorageService) UploadURL(c *gae.Context) (url *url.URL, err error) {
	return blobstore.UploadURL(c, st.UploadURLPrefix, nil)
}

func (st *BlobstoreStorageService) Type() int { return BlobstoreStorageType }

func (st *BlobstoreStorageService) HandleUpload(hc *HTTPContext) (ret []*Share) {
	ret = make([]*Share, 0)
	blobs, _, err := blobstore.ParseUpload(hc.Request)
	if err != nil {
		log.Printf("StorageService: Could not parse Upload: %s", err)
	}

	for _, bia := range blobs {
		for _, bi := range bia {
			ret = append(ret, newShare(bi))
		}
	}
	return
}

func newShare(bi *blobstore.BlobInfo) *Share {
	ret := &Share{
		StorageKey: string(bi.BlobKey),
		Expires:    DefaultValidPeriod + int64(bi.CreationTime.UnixNano()/Msec),
		Size:       bi.Size,
		MimeType:   bi.ContentType,
		Name:       bi.Filename,
		Created:    int64(bi.CreationTime.UnixNano() / Msec),
	}
	return ret
}
