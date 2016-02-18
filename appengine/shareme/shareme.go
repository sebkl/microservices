//package shareme offers an experimental google appengine example service which allows
// to share files based on a hash key which can be distributed as an URL. It demonstrates
// the  use of the blobstore and datastore functionality in combination with the gotojs package for
// appengine integration.
//
// TODOS:
//	- Fix duplicate datastore key issue
//	- Make responsive
// 	- use simple icons set for download, delete, share etc.
//	- use simple mime-type icons
//	- share button shloud copy the data url in the clipboard.
//	- Remove code noise !
// 	- create preview url ( adds the share to your session :-D)
//	- Check valid period by cronjob or on certain calls.
//	- Register escape button to unselect + ui button
//	- make responsive for mobiles.
//	- configurable file type and size limits on both, server and client side.
//	- support multiple files
//	- add send-to link (mailto:)
//	- add copy button which copies link to clipboard (!)
//	- general UI improvements
//	- consider authentication and authorization options per share.
//	- add default thumbnail for non pictures/videos
//	- implement/use url shortener (together with datastore)
//	- make storage configurable

package shareme

import (
	"appengine/datastore"
	"appengine/user"
	"encoding/json"
	"fmt"
	. "github.com/sebkl/gotojs"
	"github.com/sebkl/gotojs/gae"
	"github.com/sebkl/imgurl"
	"github.com/sebkl/microservices/gotojs/imgsrv"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultValidPeriod = 7 * 24 * 60 * 60 * 1000 //7days in msecs
	Msec               = 1000 * 1000
	KeySessionPrefix   = "key:"
	InterfaceName      = "Shareme"
)

type StorageService interface {
	UploadURL(c *gae.Context) (*url.URL, error)
	Delete(c *gae.Context, key string) error
	Stat(c *gae.Context, key string) (*Share, error)
	Get(c *gae.Context, key string) (io.ReadCloser, error)
	Thumbnail(c *gae.Context, key string, w, h int) string
	Type() int
	HandleUpload(*HTTPContext) []*Share
}

type Share struct {
	Key            string         `json:"key,omitempty" datastore:"-"`
	DSK            *datastore.Key `json:"-" datastore:"-"`
	StorageKey     string         `json:"-" datastore:"storagekey"`
	Size           int64          `json:"size,omitempty" datastore:"size"`
	Name           string         `json:"name,omitempty" datastore:"name"`
	Created        int64          `json:"created,omitempty" datastore:"created"` //msec unix
	Expires        int64          `json:"expires,omitempty" datastore:"expires"` //msec unix
	MimeType       string         `json:"mimetype,omitempty" datastore:"mimetype"`
	Error          string         `json:"error,omitempty" datastore:"-"`
	Url            string         `json:"url,omitempty" datastore:"-"`
	Thumbnail      string         `json:"thumbnail,omitempty" datastore:"-"`
	StorageService StorageService `json:"-" datastore:"-"`
	StorageType    int            `json:"storage,omitempty" datastore:"storagetype"`
}

func ReadShare(r io.Reader) *Share {
	dec := json.NewDecoder(r)
	s := &Share{}
	err := dec.Decode(s)
	if err != nil {
		log.Printf("Could not decode share: %s", err)
		return nil
	}
	return s
}

func (s *Share) IsError() bool {
	return len(s.Error) > 0
}

func (s *Share) WriteTo(w io.Writer) error {
	enc := json.NewEncoder(w)
	return enc.Encode(s)
}

func (s *Share) String() string {
	by, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(by)
}

func noShare(err error) *Share {
	log.Printf("Could generate share object: %s", err)
	return &Share{
		Error:   err.Error(),
		Size:    0,
		Expires: 0,
	}
}

type BlobBinary struct {
	io.Reader
	mimeType string
}

func (b *BlobBinary) MimeType() string {
	return b.mimeType
}

func (b *BlobBinary) Close() error {
	return nil
}

// TODO: Make actual storage configurable (Cloud storage, blobstore etc ...)
type SharemeService struct {
	storageService StorageService
}

//Thumbnail returns a 50x50 thumbnail data url of a key if possible.
func (s *SharemeService) ThumbnailURL(c *gae.Context, key string) string {
	return s.Stat(c, key).Thumbnail
}

//ImageURL returns the given object das a data url.
func (s *SharemeService) ImageURL(c *gae.Context, key string) string {
	obj := s.Get(c, key)
	if bb, ok := obj.(*BlobBinary); ok && strings.HasPrefix(bb.MimeType(), "image") {
		if url, _, err := imgurl.UrlifyR(bb, bb.MimeType(), 0, 0); err == nil {
			return url
		} else {
			panic(err)
		}
	}
	return ""
}

//MyShares returns a list of all shares that have been upload by the user.
func (s *SharemeService) MyShares(c *gae.Context, session *Session) []Share {
	ret := make([]Share, 0, len(session.Properties))
	for k, name := range session.Properties {
		if strings.HasPrefix(k, KeySessionPrefix) {
			key := strings.TrimLeft(k, KeySessionPrefix)
			share := s.Stat(c, key)

			share.Name = name
			if !share.IsError() {
				ret = append(ret, share)
			} else {
				//If there is an error with key, delete it from session.
				tdk := KeySessionPrefix + key
				session.Delete(tdk)
			}
		}
	}
	c.HTTPContext.ReturnStatus = http.StatusOK
	return ret
}

func (s *Share) Delete(c *gae.Context) (err error) {
	err = s.StorageService.Delete(c, s.StorageKey)
	if err != nil {
		return
	}
	err = datastore.Delete(c, s.DSK)
	return err
}

// Delete removes the object of a key from the storage. It wont be accessible afterwards.
func (s *SharemeService) Delete(c *gae.Context, session *Session, key string) (ret Share) {
	stat := s.Stat(c, key)
	if stat.IsError() {
		return stat
	}

	err := stat.Delete(c)
	if err != nil {
		return *noShare(err)
	}

	c.HTTPContext.ReturnStatus = http.StatusOK
	session.Delete(fmt.Sprintf("%s%s", KeySessionPrefix, key))
	ret = Share{Key: ret.Key, Expires: 0, Size: 0}
	return
}

// Upload URL generates an URL to be used for direct uploading of a blob.
func (s *SharemeService) UploadURL(c *gae.Context, cont *Container) string {
	url, err := s.storageService.UploadURL(c)
	log.Printf("Generated url :'%s'", url.String())
	if err != nil {
		log.Printf("Could not generate UpladURL %s", err)
	}

	return url.String()
}

//Get returns the blob itself.
func (s *SharemeService) Get(c *gae.Context, key string) interface{} { // Return either Share in case of Error or Binary.
	st := s.Stat(c, key)
	if st.IsError() {
		return noShare(fmt.Errorf(st.Error))
	}

	/* retrieve blob */
	rc, err := s.storageService.Get(c, st.StorageKey)
	if err != nil {
		c.HTTPContext.ReturnStatus = http.StatusNotFound
		//TODO: delete key from datastore and session.
		return noShare(err)
	}
	/* generate return object */
	return &BlobBinary{Reader: rc, mimeType: st.MimeType}
}

//Add adds a key to the users session and returns its descriptor.
func (s *SharemeService) Add(c *gae.Context, session *Session, key string) (stat Share) {
	stat = s.Stat(c, key)
	if stat.IsError() {
		return
	}
	session.Set(fmt.Sprintf("%s%s", KeySessionPrefix, key), stat.Name)
	return
}

//Stat returns the descriptor of the share.
func (s *SharemeService) Stat(c *gae.Context, key string) Share {
	dsk, err := datastore.DecodeKey(key)
	if err != nil {
		c.HTTPContext.ReturnStatus = http.StatusNotFound
		return *noShare(fmt.Errorf("Error decoding key '%s': %s", key, err))
	}
	stat := &Share{DSK: dsk}
	err = datastore.Get(c, dsk, stat)
	switch err {
	case datastore.ErrNoSuchEntity:
		c.HTTPContext.ReturnStatus = http.StatusNotFound
		return *noShare(fmt.Errorf("Entity not found for key '%s' : %s", key, err))
	default:
		c.HTTPContext.ReturnStatus = http.StatusInternalServerError
		return *noShare(fmt.Errorf("Error retrieving data for key '%s' : %s", key, err))
	case nil:
		share, err := s.storageService.Stat(c, stat.StorageKey)
		if err != nil || len(share.StorageKey) <= 0 {
			//TODO: delete from datastore
			return *noShare(fmt.Errorf("No storage key for '%s' %s", key, err))
		}
		stat.Key = key
		stat.Url = fmt.Sprintf("%s/%s/%s/%s", c.HTTPContext.Container.BaseUrl(), InterfaceName, "Get", key)
		stat.Thumbnail = s.storageService.Thumbnail(c, stat.StorageKey, 50, 50)
		stat.StorageService = s.storageService
		return *stat
	}
}

//HandleUpload is a method that deals with full form submits. Use the url of this binding as POST action for the
//embedded Form
func (s *SharemeService) HandleUpload(hc *HTTPContext, c *gae.Context, session *Session) []*Share {
	ret := make([]*Share, 0)
	bis := s.storageService.HandleUpload(hc)
	for _, stat := range bis {
		dsk, err := datastore.Put(c, datastore.NewIncompleteKey(c, "Share", nil), stat)
		if err != nil {
			//TODO: delete blob
			continue
		}
		fkey := dsk.Encode()
		session.Set(fmt.Sprintf("%s%s", KeySessionPrefix, fkey), stat.Name)
		rstat := s.Stat(c, fkey)
		ret = append(ret, &rstat)

	}
	return ret
}

// HandleCleanup removes all outdated elements based on their validity period.
func HandleCleanup(w http.ResponseWriter, r *http.Request) {
	hc := NewHTTPContext(r, w) // HTTPContext
	c := gae.NewContext(hc)    //GAE Context
	stamp := int64(time.Now().UnixNano() / Msec)
	q := datastore.NewQuery("Share").Filter("expires >", stamp)

	var shares []Share
	keys, err := q.GetAll(c, &shares)
	if err != nil {
		log.Printf("Error during cleanip: %s", err)
		return
	}

	if len(keys) > 0 {
		log.Printf("Found %d expired shares", len(shares))
		for i, key := range keys {
			shares[i].DSK = key
			shares[i].Delete(c)
			log.Printf("Share '%s' deleted", shares[i].Key)
		}
	}
}

//NewSharemeService creates a new service instance.
func NewSharemeService(ak ...string) (ret *SharemeService) {
	ret = &SharemeService{
		storageService: &BlobstoreStorageService{},
	}
	return
}

//RestrictToAdmin is a GOTOJS filter that allow to restrict certain calls to the administrators
// of the app engine module/app
func RestrictToAdmin(c *gae.Context) bool {
	if c == nil {
		c.HTTPContext.ReturnStatus = http.StatusInternalServerError
		log.Printf("Could not retrieve context.")
		return false
	}

	if u := user.Current(c); u != nil {
		return user.IsAdmin(c)
	} else {
		c.HTTPContext.ReturnStatus = http.StatusForbidden
		return false
	}
}

//init initializes the app engine module.
func init() {
	log.Printf("INIT()")
	// This key must be the same across all GAE instances in order to decrypt session data
	c := NewContainer(Properties{P_APPLICATIONKEY: "APPKEY_CHANGEME_________________"})

	c.Redirect("/", "/p/")

	sms := NewSharemeService()
	c.ExposeInterface(sms, InterfaceName)
	c.ExposeInterface(&imgsrv.ImageService{}, "Image")
	c.EnableFileServer("htdocs", "p")

	ub, _ := c.Binding(InterfaceName, "HandleUpload")
	sms.storageService = &BlobstoreStorageService{ub.Url().Path}

	c.ExposeYourself()

	gae.SetupAndStart(c)
	http.HandleFunc("/cleanup", HandleCleanup)
}
