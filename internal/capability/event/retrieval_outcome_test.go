package event

import (
 "errors"
 "testing"
)

func TestRetrievalOutcome(t *testing.T) {
 cases:=[]struct{status AcquisitionStatus; err error; want string}{
 {AcquisitionSucceeded,nil,"SUCCEEDED"},
 {AcquisitionPartial,nil,"PARTIAL"},
 {AcquisitionTimeout,nil,"TIMEOUT"},
 {AcquisitionFailed,nil,"FAILED"},
 {AcquisitionSucceeded,errors.New("provider secret must not be exposed"),"FAILED"},
 }
 for _,tc:=range cases {
  if got:=retrievalOutcome(tc.status,tc.err);got!=tc.want {t.Fatalf("%q: got %q want %q",tc.status,got,tc.want)}
 }
}
