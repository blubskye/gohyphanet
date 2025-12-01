package requests

import "fmt"

// RequestStatus represents the status of a request
type RequestStatus int

const (
	// StatusNotFinished means the request is still in progress
	StatusNotFinished RequestStatus = -1

	// StatusSuccess means the request succeeded
	StatusSuccess RequestStatus = 0

	// StatusRouteNotFound means no suitable peer could be found
	StatusRouteNotFound RequestStatus = 1

	// StatusDataNotFound means the data doesn't exist in the network
	StatusDataNotFound RequestStatus = 3

	// StatusTransferFailed means data transfer failed
	StatusTransferFailed RequestStatus = 4

	// StatusVerifyFailure means data verification failed
	StatusVerifyFailure RequestStatus = 5

	// StatusTimedOut means the request timed out
	StatusTimedOut RequestStatus = 6

	// StatusRejectedOverload means the request was rejected due to overload
	StatusRejectedOverload RequestStatus = 7

	// StatusInternalError means an internal error occurred
	StatusInternalError RequestStatus = 8

	// StatusRecentlyFailed means multiple nodes recently failed for this key
	StatusRecentlyFailed RequestStatus = 9

	// StatusGetOfferVerifyFailure means an offered key failed verification
	StatusGetOfferVerifyFailure RequestStatus = 10

	// StatusGetOfferTransferFailed means an offered key transfer failed
	StatusGetOfferTransferFailed RequestStatus = 11
)

// String returns a string representation of the status
func (rs RequestStatus) String() string {
	switch rs {
	case StatusNotFinished:
		return "NotFinished"
	case StatusSuccess:
		return "Success"
	case StatusRouteNotFound:
		return "RouteNotFound"
	case StatusDataNotFound:
		return "DataNotFound"
	case StatusTransferFailed:
		return "TransferFailed"
	case StatusVerifyFailure:
		return "VerifyFailure"
	case StatusTimedOut:
		return "TimedOut"
	case StatusRejectedOverload:
		return "RejectedOverload"
	case StatusInternalError:
		return "InternalError"
	case StatusRecentlyFailed:
		return "RecentlyFailed"
	case StatusGetOfferVerifyFailure:
		return "GetOfferVerifyFailure"
	case StatusGetOfferTransferFailed:
		return "GetOfferTransferFailed"
	default:
		return fmt.Sprintf("Unknown(%d)", rs)
	}
}

// IsTerminal returns whether this status is terminal (request is done)
func (rs RequestStatus) IsTerminal() bool {
	return rs != StatusNotFinished
}

// IsSuccess returns whether this status represents success
func (rs RequestStatus) IsSuccess() bool {
	return rs == StatusSuccess
}

// IsFailure returns whether this status represents a failure
func (rs RequestStatus) IsFailure() bool {
	return rs.IsTerminal() && !rs.IsSuccess()
}
