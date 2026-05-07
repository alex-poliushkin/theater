package thtrlsp

import authoringthtr "github.com/alex-poliushkin/theater/internal/authoring/thtr"

func authoringFormatBytes(data []byte) ([]byte, error) {
	return authoringthtr.Format(data)
}
