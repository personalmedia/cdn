package processor

import (
	_ "embed"
	"image"
	"math"

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
)

//go:embed cascades/facefinder
var cascadeData []byte

var faceClassifier *pigo.Pigo

func init() {
	var err error
	p := pigo.NewPigo()
	faceClassifier, err = p.Unpack(cascadeData)
	if err != nil {
		panic("failed to unpack embedded facefinder cascade: " + err.Error())
	}
}

// DetectFace runs the Pigo classifier and returns the bounding box
// of the largest detected face. Returns false if no face is found.
func DetectFace(img image.Image) (image.Rectangle, bool) {
	if faceClassifier == nil {
		return image.Rect(0, 0, 0, 0), false
	}

	src := pigo.ImgToNRGBA(img)
	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Max.X, src.Bounds().Max.Y

	cParams := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	// Run cascade over image
	dets := faceClassifier.RunCascade(cParams, 0.0)
	
	// Cluster detections (IoU threshold 0.2)
	dets = faceClassifier.ClusterDetections(dets, 0.2)

	if len(dets) == 0 {
		return image.Rect(0, 0, 0, 0), false
	}

	// Find the most prominent face (largest scale / highest score combination)
	var bestFace pigo.Detection
	var bestScore float32 = -1.0

	for _, det := range dets {
		if det.Q >= 5.0 { // Minimum quality threshold to avoid false positives
			// The score could be a mix of scale and detection quality
			score := float32(det.Scale) * float32(det.Q) 
			if score > bestScore {
				bestScore = score
				bestFace = det
			}
		}
	}

	if bestScore < 0 {
		return image.Rect(0, 0, 0, 0), false
	}

	// Pigo returns row, col (center) and scale (diameter)
	radius := bestFace.Scale / 2
	rect := image.Rect(
		bestFace.Col-radius,
		bestFace.Row-radius,
		bestFace.Col+radius,
		bestFace.Row+radius,
	)

	return rect, true
}

func calculateFaceCrop(bounds image.Rectangle, faceRect image.Rectangle, targetW, targetH int) image.Rectangle {
	// If the original image is already the exact target ratio, just return bounds
	origRatio := float64(bounds.Dx()) / float64(bounds.Dy())
	targetRatio := float64(targetW) / float64(targetH)

	if math.Abs(origRatio-targetRatio) < 0.01 {
		return bounds // Aspect ratio is identical
	}

	var cropW, cropH int

	if origRatio > targetRatio {
		// Image is too wide; crop width, keep full height
		cropH = bounds.Dy()
		cropW = int(float64(cropH) * targetRatio)
	} else {
		// Image is too tall; crop height, keep full width
		cropW = bounds.Dx()
		cropH = int(float64(cropW) / targetRatio)
	}

	// Attempt to center the crop box around the face
	faceCenterX := faceRect.Min.X + faceRect.Dx()/2
	faceCenterY := faceRect.Min.Y + faceRect.Dy()/2

	cropX := faceCenterX - cropW/2
	cropY := faceCenterY - cropH/2

	// Clamp to image boundaries
	if cropX < 0 {
		cropX = 0
	}
	if cropY < 0 {
		cropY = 0
	}
	if cropX+cropW > bounds.Dx() {
		cropX = bounds.Dx() - cropW
	}
	if cropY+cropH > bounds.Dy() {
		cropY = bounds.Dy() - cropH
	}

	return image.Rect(cropX, cropY, cropX+cropW, cropY+cropH)
}

func ProcessPortraitWithFaceDetect(img image.Image, w, h int) image.Image {
	if w == 0 && h == 0 {
		return img
	}

	if w == 0 || h == 0 {
		return imaging.Resize(img, w, h, imaging.Lanczos)
	}

	// 1. Detect faces
	faceRect, ok := DetectFace(img)

	if ok {
		// 2. We found a face! Crop maintaining the target WxH ratio anchored on the face
		cropRect := calculateFaceCrop(img.Bounds(), faceRect, w, h)
		img = imaging.Crop(img, cropRect)

		// The image is now the correct aspect ratio, so a standard resize 
		// (without cropping) will perfectly fit w x h without distortion.
		return imaging.Resize(img, w, h, imaging.Lanczos)
	}

	// 3. Fallback if no face is detected: Top fill
	return imaging.Fill(img, w, h, imaging.Top, imaging.Lanczos)
}
