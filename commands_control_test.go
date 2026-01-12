package main

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestDecodeCustomer(t *testing.T) {
	id := uuid.New().String()
	direct := []byte(`{"customer":{"id":"` + id + `","external_id":"ext_1","email":"test@example.com"}}`)
	cust, err := decodeCustomer(direct)
	if err != nil {
		t.Fatalf("decode direct: %v", err)
	}
	if cust.Customer.Id == nil || formatUUIDPtr(cust.Customer.Id) != id {
		t.Fatalf("unexpected customer id: %v", cust.Customer.Id)
	}

	envelope := []byte(`{"customer":{"customer":{"id":"` + id + `","external_id":"ext_2","email":"test@example.com"}}}`)
	cust, err = decodeCustomer(envelope)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if cust.Customer.ExternalId == nil || *cust.Customer.ExternalId != "ext_2" {
		t.Fatalf("unexpected external id")
	}
}

func TestDecodeCustomerMissing(t *testing.T) {
	_, err := decodeCustomer(bytes.TrimSpace([]byte(`{}`)))
	if err == nil {
		t.Fatalf("expected error for missing customer payload")
	}
}
