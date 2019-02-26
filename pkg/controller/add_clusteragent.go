package controller

import (
	"github.com/sjeltuhin/appdynamics-operator/pkg/controller/clusteragent"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, clusteragent.Add)
}
