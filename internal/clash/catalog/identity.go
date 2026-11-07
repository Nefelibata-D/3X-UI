package catalog

import (
	"errors"
	"fmt"
)

const LocalNodeKey = "local"

func NodeKey(nodeID int) string { return fmt.Sprintf("node:%d", nodeID) }

var ErrClientNotFound = errors.New("client not found")
