// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package h2rev2

import "testing"

func Test_serverURL(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		id      string
		want    string
		wantErr bool
	}{
		{
			name: "valid",
			host: "https://host:9443/base",
			id:   "dialer001",
			want: "https://host:9443/base/revdial?id=dialer001",
		},
		{
			name:    "invalid host scheme",
			host:    "http://host:9443/base",
			id:      "dialer001",
			wantErr: true,
		},
		{
			name:    "invalid host port",
			host:    "https://host:port/base",
			id:      "dialer001",
			wantErr: true,
		},
		{
			name:    "empty id",
			host:    "https://host:9443/base",
			id:      "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serverURL(tt.host, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("serverURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("serverURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
