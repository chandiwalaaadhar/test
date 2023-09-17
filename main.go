package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/klauspost/reedsolomon"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func encodeDataToImage(data interface{}) (image.Image, error) {
	dataBytes, err := marshalDataToBytes(data)
	if err != nil {
		return nil, err
	}

	dataBytesWithLength := prependLengthToData(dataBytes)

	encodedShards, err := encodeDataUsingReedSolomon(dataBytesWithLength)
	if err != nil {
		return nil, err
	}

	flattenedData := mergeShards(encodedShards)

	return createImageFromData(flattenedData), nil
}

func marshalDataToBytes(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

func prependLengthToData(dataBytes []byte) []byte {
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(dataBytes)))

	return append(lengthBytes, dataBytes...)
}

func encodeDataUsingReedSolomon(dataBytes []byte) ([][]byte, error) {
	encoder, err := reedsolomon.New(4, 2)
	if err != nil {
		return nil, err
	}

	shards, err := encoder.Split(dataBytes)
	if err != nil {
		return nil, err
	}

	if err = encoder.Encode(shards); err != nil {
		return nil, err
	}

	return shards, nil
}

func mergeShards(shards [][]byte) []byte {
	totalPixels := 0
	for _, shard := range shards {
		totalPixels += len(shard)
	}

	flattenedData := make([]byte, 0, totalPixels)
	for _, shard := range shards {
		flattenedData = append(flattenedData, shard...)
	}

	return flattenedData
}

func createImageFromData(flattenedData []byte) image.Image {
	totalPixels := len(flattenedData)
	sideLength := int(math.Ceil(math.Sqrt(float64(totalPixels))))

	dc := gg.NewContext(sideLength, sideLength)

	for i := 0; i < totalPixels; i++ {
		x := i % sideLength
		y := i / sideLength
		dc.SetColor(color.Gray{Y: flattenedData[i]})
		dc.SetPixel(x, y)
	}

	return dc.Image()
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
