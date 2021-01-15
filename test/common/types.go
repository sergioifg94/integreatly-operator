package common

import (
	"strings"

	"github.com/integr8ly/integreatly-operator/pkg/resources"
)

var (
	NamespacePrefix = GetNamespacePrefix()
)

func GetNamespacePrefix() string {
	ns, err := resources.GetWatchNamespace()
	if err != nil {
		return ""
	}
	return strings.Join(strings.Split(ns, "-")[0:2], "-") + "-"

}
