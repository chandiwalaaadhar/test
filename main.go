package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fogleman/gg"
	"github.com/klauspost/reedsolomon"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func encodeDataToImage(data interface{}) (image.Image, error) {
	// Convert the data into a byte slice using JSON encoding.
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Create a byte slice of length 4 to store the length of the dataBytes.
	// This is useful during decoding to know how much data to read.
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(dataBytes)))
	// Prepend this length information to the actual data.
	dataBytes = append(lengthBytes, dataBytes...)

	// Create a new Reed-Solomon encoder with 4 data shards and 2 parity shards.
	enc, err := reedsolomon.New(4, 2)
	if err != nil {
		return nil, err
	}

	// Split the data into shards.
	shards, err := enc.Split(dataBytes)
	if err != nil {
		return nil, err
	}

	// Encode the shards. This creates parity shards and may modify the data shards.
	if err = enc.Encode(shards); err != nil {
		return nil, err
	}

	// Calculate the total number of pixels needed to represent the data in an image.
	// The image is planned be a square (it can be made in any shape), so need to find out its side length.
	totalPixels := len(dataBytes)
	// The side length is the ceiling of the square root of totalPixels.
	sideLength := int(math.Ceil(math.Sqrt(float64(totalPixels))))
	dc := gg.NewContext(sideLength, sideLength) // Create a new drawing context for the image.

	// Merge the shards into a single byte for easier pixel-wise processing.
	flattenedData := make([]byte, 0, totalPixels)
	for _, shard := range shards {
		flattenedData = append(flattenedData, shard...)
	}

	// Set each pixel's color based on the byte values in flattenedData.
	for i := 0; i < totalPixels; i++ {
		x := i % sideLength                          // x-coordinate in the image.
		y := i / sideLength                          // y-coordinate in the image.
		dc.SetColor(color.Gray{Y: flattenedData[i]}) // Set the color for the pixel.
		dc.SetPixel(x, y)                            // Draw the pixel.
	}

	return dc.Image(), nil
}

func decodeImageToData(img image.Image) (map[string]interface{}, error) {
	// Get the boundaries of the image.
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	// Initialize a byte slice to extract byte data from the image.
	extractedBytes := make([]byte, 0, width*height)

	// Loop through each pixel in the image.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Convert the pixel color to grayscale.
			grayColor := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			// Append the grayscale value (Y channel) to the byte slice.
			extractedBytes = append(extractedBytes, grayColor.Y)
		}
	}

	// The first 4 bytes represent the length of the actual data.
	if len(extractedBytes) < 4 {
		return nil, errors.New("insufficient data in image")
	}
	// Convert the 4 bytes to an integer value.
	dataLength := binary.BigEndian.Uint32(extractedBytes[:4])
	// The remaining bytes after the first 4 are the encoded data.
	encodedDataBytes := extractedBytes[4:]

	// If the data is shorter than expected, return an error.
	if int(dataLength) > len(encodedDataBytes) {
		return nil, errors.New("data in image is shorter than expected")
	}

	// Use the Reed-Solomon encoder to split the data back into shards.
	enc, err := reedsolomon.New(4, 2)
	if err != nil {
		return nil, err
	}
	shards, err := enc.Split(encodedDataBytes)
	if err != nil {
		return nil, err
	}

	// Attempt to reconstruct the original shards, in case any are missing or corrupted.
	if err := enc.Reconstruct(shards); err != nil {
		return nil, err
	}

	// Merge the reconstructed shards to rebuild the original byte data.
	rebuiltData := make([]byte, 0, dataLength)
	for _, shard := range shards {
		rebuiltData = append(rebuiltData, shard...)
	}
	// Trim any excess bytes from the end.
	rebuiltData = rebuiltData[:dataLength]

	// Convert the byte data back into a map.
	var result map[string]interface{}
	if err := json.Unmarshal(rebuiltData, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func readImageFromPath(filePath string) image.Image {
	file, err := os.Open(filePath) // Replace "image.jpg" with the path to your image file
	if err != nil {
		fmt.Println("Error opening image file:", err)
		return nil
	}
	defer file.Close()

	// Decode the image
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Println("Error decoding image:", err)
		return nil
	}
	return img

}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("Encode: go run main.go encode \"Your text here\" [filename]")
		fmt.Println("Decode: go run main.go decode filename.png")
		return
	}

	action := os.Args[1]
	filename := "output.png"

	switch action {
	case "encode":
		text := os.Args[2]
		if len(os.Args) > 3 {
			filename = os.Args[3]
			// if filename contains no extension, add .png
			if len(filename) < 4 || filename[len(filename)-4:] != ".png" {
				filename += ".png"
			}
		}

		data := map[string]interface{}{
			"Data": text,
		}

		// Encode data to image
		img, err := encodeDataToImage(data)
		if err != nil {
			fmt.Println("Error encoding data:", err)
			return
		}

		// Save image as PNG
		file, _ := os.Create(filename)
		err = png.Encode(file, img)

		if err != nil {
			fmt.Println("Error saving as png:", err)
			return
		}

		err = file.Close()
		if err != nil {
			fmt.Println("Error closing file:", err)
			return
		}

	case "decode":
		filename = os.Args[2]

		// Decode data from image
		decodedData, err := decodeImageToData(readImageFromPath(filename))
		if err != nil {
			fmt.Println("Error decoding data:", err)
			return
		}

		fmt.Println("Decoded data:", decodedData["Data"])

	default:
		fmt.Println("Invalid action. Use 'encode' or 'decode'.")
	}
}
