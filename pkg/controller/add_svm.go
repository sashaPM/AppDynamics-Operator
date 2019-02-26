package controller

import (
	"github.com/sjeltuhin/appdynamics-operator/pkg/controller/svm"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, svm.Add)
}
