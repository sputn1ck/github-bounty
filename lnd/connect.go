package lnd

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
	"net/url"
	"time"
)



// ConnectFromLndConnectWithTimeout uses ConnectFromLndConnect to
// connect to a lnd node but also aborts after a given timeout duration.
func ConnectFromLndConnectWithTimeout(ctx context.Context, lndConnectUri string, timeout time.Duration) (*grpc.ClientConn, error) {
	ctx,cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cc, err := ConnectFromLndConnect(ctx, lndConnectUri)
	if err != nil {
		return nil, err
	}
	return cc,nil
}

// ConnectFromLndConnect uses a lnd connect uri string, containing
// host, macaroon and optional credentials to connect to a lnd node.
func ConnectFromLndConnect(ctx context.Context, lndConnectUri string) (*grpc.ClientConn, error) {
	uri := &url.URL{}
	uri, err := uri.Parse(lndConnectUri)
	if err != nil {
		return nil, err
	}

	address, mac, tlsCreds, err := UnmarshalLndConnectURI(uri)
	if err != nil {
		return nil, err
	}

	macCred := newMacaroonCredential(mac)

	dialOpts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(macCred),
		grpc.WithBlock(),
	}

	if tlsCreds != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(*tlsCreds))
	}

	return grpc.DialContext(ctx, address, dialOpts...)
}
// newMacaroonCredential returns a copy of the passed macaroon wrapped in a
// macaroonCredential struct which implements PerRPCCredentials.
func newMacaroonCredential(m *macaroon.Macaroon) macaroonCredential {
	ms := macaroonCredential{}
	ms.Macaroon = m.Clone()
	return ms
}

// macaroonCredential wraps a macaroon to implement the
// credentials.PerRPCCredentials interface.
type macaroonCredential struct {
	*macaroon.Macaroon
}

// RequireTransportSecurity implements the PerRPCCredentials interface.
func (m macaroonCredential) RequireTransportSecurity() bool {
	return true
}

// GetRequestMetadata implements the PerRPCCredentials interface. This method
// is required in order to pass the wrapped macaroon into the gRPC context.
// With this, the macaroon will be available within the request handling scope
// of the ultimate gRPC server implementation.
func (m macaroonCredential) GetRequestMetadata(ctx context.Context,
	uri ...string) (map[string]string, error) {

	macBytes, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}

	md := make(map[string]string)
	md["macaroon"] = hex.EncodeToString(macBytes)
	return md, nil
}


// UnmarshalLndConnectURI takes a lndconnect uri
// (https://github.com/LN-Zap/lndconnect/blob/master/lnd_connect_uri.md)
// and parses it into the address the macaroon and optionally
// the credentials, if provided.
func UnmarshalLndConnectURI(uri *url.URL) (string, *macaroon.Macaroon, *credentials.TransportCredentials, error) {
	var address string
	address = uri.Host

	macar := &macaroon.Macaroon{}
	if mac, ok := uri.Query()["macaroon"]; ok {
		if len(mac) != 1 {
			return "", nil, nil, fmt.Errorf("unable to get macaroon from uri")
		}
		macBytes, err := base64.RawURLEncoding.DecodeString(mac[0])
		if err != nil {
			return "", nil, nil, fmt.Errorf("unable to decode base64 macaroon: %v", err)
		}
		err = macar.UnmarshalBinary(macBytes)
		if err != nil {
			return "", nil, nil, fmt.Errorf("unable to unmarshal binary macaroon: %v", err)
		}
	}

	var creds credentials.TransportCredentials
	if cert, ok := uri.Query()["cert"]; ok {
		switch len(cert) {
		case 0:
			break
		case 1:
			certStr, err := reconstructCertFromUrlBase(cert[0])
			if err != nil {
				return "", nil, nil, fmt.Errorf("unable to decode url base: %v", err)
			}
			certBytes := []byte(certStr)
			pool := x509.NewCertPool()
			if ok := pool.AppendCertsFromPEM(certBytes); !ok {
				return "", nil, nil, fmt.Errorf("unable to append pem cert: %v", ok)
			}
			creds = credentials.NewClientTLSFromCert(pool, "")
		default:
			return "", nil, nil, fmt.Errorf("expected len 1 or 0")
		}
	}
	return address, macar, &creds, nil
}

func reconstructCertFromUrlBase(str string) (string, error) {
	out, err := base64UrlToBase64(str)
	if err != nil {
		return "", nil
	}

	lines := int(len(out) / 64)
	for i := 1; i <= lines; i++ {
		out = out[:(i*64)+(i-1)] + "\n" + out[(i*64)+(i-1):]
	}

	return fmt.Sprintf("\n-----BEGIN CERTIFICATE-----\n%s\n-----END CERTIFICATE-----\n", out), nil
}

func base64UrlToBase64(str string) (string, error) {
	urlBase, err := base64.RawURLEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(urlBase), nil

}