package fcp_server

// FCPMessage represents an FCP protocol message
type FCPMessage struct {
	Name   string
	Fields map[string]string
	Data   []byte
}

// Protocol error codes (matching Java implementation)
const (
	ProtocolErrorMessageParseError           = 1
	ProtocolErrorMissingField                 = 2
	ProtocolErrorInvalidField                 = 25
	ProtocolErrorFreenetURIParseError         = 4
	ProtocolErrorClientHelloMustBeFirst       = 18
	ProtocolErrorNoLateClientHellos           = 19
	ProtocolErrorIdentifierCollision          = 5
	ProtocolErrorUnknownNodeIdentifier        = 6
	ProtocolErrorDiskTargetExists             = 7
	ProtocolErrorCouldNotCreateFile           = 8
	ProtocolErrorAccessDenied                 = 9
	ProtocolErrorDirectoryNotAllowed          = 10
	ProtocolErrorFileNotAllowed               = 11
	ProtocolErrorGetFailed                    = 12
	ProtocolErrorPutFailed                    = 13
	ProtocolErrorShuttingDown                 = 14
	ProtocolErrorInternalError                = 15
	ProtocolErrorTooManyActiveRequests        = 16
	ProtocolErrorFileTooBig                   = 17
	ProtocolErrorErrorParsingNumber           = 27
	ProtocolErrorCouldNotReadFile             = 28
)

// GetProtocolErrorDescription returns a human-readable description of the error code
func GetProtocolErrorDescription(code int) string {
	switch code {
	case ProtocolErrorMessageParseError:
		return "Message parse error"
	case ProtocolErrorMissingField:
		return "Missing field"
	case ProtocolErrorInvalidField:
		return "Invalid field"
	case ProtocolErrorFreenetURIParseError:
		return "Freenet URI parse error"
	case ProtocolErrorClientHelloMustBeFirst:
		return "ClientHello must be first message"
	case ProtocolErrorNoLateClientHellos:
		return "No late ClientHellos"
	case ProtocolErrorIdentifierCollision:
		return "Identifier collision"
	case ProtocolErrorUnknownNodeIdentifier:
		return "Unknown node identifier"
	case ProtocolErrorDiskTargetExists:
		return "Disk target exists"
	case ProtocolErrorCouldNotCreateFile:
		return "Could not create file"
	case ProtocolErrorAccessDenied:
		return "Access denied"
	case ProtocolErrorDirectoryNotAllowed:
		return "Directory not allowed"
	case ProtocolErrorFileNotAllowed:
		return "File not allowed"
	case ProtocolErrorGetFailed:
		return "Get failed"
	case ProtocolErrorPutFailed:
		return "Put failed"
	case ProtocolErrorShuttingDown:
		return "Shutting down"
	case ProtocolErrorInternalError:
		return "Internal error"
	case ProtocolErrorTooManyActiveRequests:
		return "Too many active requests"
	case ProtocolErrorFileTooBig:
		return "File too big"
	case ProtocolErrorErrorParsingNumber:
		return "Error parsing number"
	case ProtocolErrorCouldNotReadFile:
		return "Could not read file"
	default:
		return "Unknown error"
	}
}

// Request status codes
const (
	RequestStatusPending    = "Pending"
	RequestStatusRunning    = "Running"
	RequestStatusSuccess    = "Success"
	RequestStatusFailed     = "Failed"
	RequestStatusDataFound  = "DataFound"
	RequestStatusDataNotFound = "DataNotFound"
)

// FCPRequest tracks an active FCP request
type FCPRequest struct {
	Identifier string
	URI        string
	Status     string
	StartTime  int64
	Priority   int16
	Global     bool
	cancel     chan struct{}
}

// Cancel cancels the request
func (r *FCPRequest) Cancel() {
	close(r.cancel)
}
