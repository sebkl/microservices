package imgsrv

import (
	"github.com/sebkl/imgurl"
	"github.com/sebkl/go-nude"
	. "github.com/sebkl/gotojs"
)

type ImageService struct { }

func (s *ImageService) Urlify(img *BinaryContent,maxwidth,maxheight int) string{
	ret,_,err := imgurl.UrlifyR(img,img.MimeType(),maxwidth,maxheight)
	if err != nil {
		panic(err)
	}

	return ret
}

func (s *ImageService) IsNude(bc *BinaryContent) bool {
	img, err := imgurl.Decode(bc,bc.MimeType())
	if err != nil {
		return true
	}

	isNude, err := nude.IsImageNude(img)
	if err != nil {
		return true
	}

	return isNude
}
