package shareme


import (
	. "github.com/sebkl/gotojs"
	"github.com/sebkl/gotojs/gae"
	"log"
	"io"
	"appengine"
	"appengine/blobstore"
	"fmt"
	"net/http"
	"io/ioutil"
	"bytes"
	"mime/multipart"
	"github.com/sebkl/imgurl"
)

const (
	FileAPI = iota
	UploadURL
)


type BlobstoreStorageService struct {}
type BlobstoreUploadURL struct {
	BlobstoreStorageService
}
type BlobstoreFileAPI struct {
	BlobstoreStorageService
}

func (st *BlobstoreStorageService) Thumbnail(c *gae.Context, key string, w,h int) (ret string) {
	stat, err := st.Stat(c,key)
	if err != nil {
		log.Printf("Blobstore.Thumbnail '%s': %s",key,err)
		return
	}

	reader, err := st.Get(c,key)
	if err != nil {
		log.Printf("Blobstore.Thumbnail '%s': %s",key,err)
		return
	}

	ret,_,err = imgurl.UrlifyR(reader,stat.MimeType,w,h)
	return
}

func (st *BlobstoreStorageService) Get(c *gae.Context, key string) (ret io.ReadCloser, err error) {
	k := appengine.BlobKey(key)
	_,err = blobstore.Stat(c,k)
	if err == nil {
		ret = ioutil.NopCloser(blobstore.NewReader(c,k))
	}
	return
}

func (st *BlobstoreStorageService) Stat(c *gae.Context, key string) (*Share,error) {
	bi,err := blobstore.Stat(c,appengine.BlobKey(key))
	stat := newShare(bi)
	return stat,err
}

func (st *BlobstoreStorageService) Delete(c *gae.Context,key string) (err error) {
	_,err = st.Stat(c,key)
	if err != nil {
		return
	}

	err = blobstore.Delete(c,appengine.BlobKey(key))
	return
}


func (st *BlobstoreFileAPI) Put(s *SharemeService,c *gae.Context, bc *BinaryContent, name string) (key string, err error) {
	// ------------------------------------------------------------
	// Thanks google for removing the File API. I'll keep the below
	// code to remember good times.
	// TODO: Remove once File api has been removed.
	writer, err := blobstore.Create(c, bc.MimeType())
	if err != nil {
		return
	}

	bi,err := io.Copy(writer,bc);
	if err != nil {
		return
	}

	writer.Close()
	bc.Close()

	bkey, err := writer.Key()
	key = string(bkey)
	log.Printf("BLOBSTORE created %d bytes [%s] (using FileAPI)",bi,key)
	return
}

func (st *BlobstoreFileAPI) Type() int { return FileAPI }

func (st *BlobstoreUploadURL) Put(s *SharemeService,c *gae.Context,bc *BinaryContent, name string) (key string, err error) {
	//user := user.Current(c)

	// ------------------------------------------------------------
	// Beware of ugliness !!!
	// Build upload URL, and call it ourselves. Workaround for 
	// deprecated file API
	bdry := Encoding.EncodeToString(GenerateKey(10)) // Generate random boundary.
	url, err := blobstore.UploadURL(c, s.uploadURL(), nil)
	if err != nil {
		return
	}

	req,err  := http.NewRequest("POST",url.String(),nil)

	req.Header.Set("Content-Type","multipart/form-data; boundary=" + bdry)

	tb := bytes.NewBufferString("")
	wr:= multipart.NewWriter(tb)
	wr.SetBoundary(bdry)

	mimeheader := make(map[string][]string)
	mimeheader["Content-Type"] = []string{bc.MimeType()}
	mimeheader["Content-Disposition"] = []string{"form-data; name=\"content\"; filename=\"" + name + "\""}
	bcwr,err := wr.CreatePart(mimeheader)
	bi, err := io.Copy(bcwr,bc)
	if err != nil {
		return
	}

	err = wr.Close()
	req.Body = ioutil.NopCloser(tb)
	//req.Header.Set("shareme-auth":

	resp, err := c.Client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	sh := ReadShare(resp.Body)
	if sh == nil {
		err = fmt.Errorf("Could not read share from internal upload url.")
		return
	}

	key = sh.Key
	log.Printf("BLOBSTORE created %d bytes [%s] (using UploadURL)",bi,key)
	return
}

func (st *BlobstoreUploadURL) Type() int { return UploadURL }


func newShare(bi *blobstore.BlobInfo) *Share {
	ret := &Share{
		Key: string(bi.BlobKey),
		Expires: DefaultValidPeriod + int64(bi.CreationTime.UnixNano() / Msec),
		Size: bi.Size,
		MimeType: bi.ContentType,
		Name: bi.Filename,
		Created: int64(bi.CreationTime.UnixNano() / Msec),
	}
	return ret
}

// HandleUpload internally handles uploads
// TODO: Do some kind of authentication here
func HandleUpload(w http.ResponseWriter, r *http.Request)  {
	//c := appengine.NewContext(r)
	blobs, _, _ := blobstore.ParseUpload(r)

	// TODO: Deal with multiple blobs
	for _,b := range blobs {
		s := newShare(b[0])
		w.Header().Set("Content-Type","application/json")
		s.WriteTo(w)
		return
	}

	w.Header().Set("Content-Type","text/plain")
	return
}
