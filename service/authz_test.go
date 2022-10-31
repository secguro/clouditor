// Copyright 2021-2022 Fraunhofer AISEC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
//           $$\                           $$\ $$\   $$\
//           $$ |                          $$ |\__|  $$ |
//  $$$$$$$\ $$ | $$$$$$\  $$\   $$\  $$$$$$$ |$$\ $$$$$$\    $$$$$$\   $$$$$$\
// $$  _____|$$ |$$  __$$\ $$ |  $$ |$$  __$$ |$$ |\_$$  _|  $$  __$$\ $$  __$$\
// $$ /      $$ |$$ /  $$ |$$ |  $$ |$$ /  $$ |$$ |  $$ |    $$ /  $$ |$$ | \__|
// $$ |      $$ |$$ |  $$ |$$ |  $$ |$$ |  $$ |$$ |  $$ |$$\ $$ |  $$ |$$ |
// \$$$$$$\  $$ |\$$$$$   |\$$$$$   |\$$$$$$  |$$ |  \$$$   |\$$$$$   |$$ |
//  \_______|\__| \______/  \______/  \_______|\__|   \____/  \______/ \__|
//
// This file is part of Clouditor Community Edition.

package service

import (
	"context"
	"reflect"
	"testing"

	"clouditor.io/clouditor/api/discovery"
	"clouditor.io/clouditor/api/orchestrator"
	"clouditor.io/clouditor/internal/testutil"
)

func TestAuthorizationStrategyAllowAll_CheckAccess(t *testing.T) {
	type args struct {
		ctx context.Context
		typ RequestType
		req orchestrator.CloudServiceRequest
	}
	tests := []struct {
		name string
		a    *AuthorizationStrategyAllowAll
		args args
		want bool
	}{
		{
			name: "always true",
			a:    &AuthorizationStrategyAllowAll{},
			args: args{
				ctx: context.Background(),
				typ: AccessCreate,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: discovery.DefaultCloudServiceID},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AuthorizationStrategyAllowAll{}
			if got := a.CheckAccess(tt.args.ctx, tt.args.typ, tt.args.req); got != tt.want {
				t.Errorf("AuthorizationStrategyAllowAll.CheckAccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizationStrategyAllowAll_AllowedCloudServices(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name     string
		a        *AuthorizationStrategyAllowAll
		args     args
		wantAll  bool
		wantList []string
	}{
		{
			name:     "all allowed",
			a:        &AuthorizationStrategyAllowAll{},
			args:     args{},
			wantAll:  true,
			wantList: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AuthorizationStrategyAllowAll{}
			gotAll, gotList := a.AllowedCloudServices(tt.args.ctx)
			if gotAll != tt.wantAll {
				t.Errorf("AuthorizationStrategyAllowAll.AllowedCloudServices() got = %v, want %v", gotAll, tt.wantAll)
			}
			if !reflect.DeepEqual(gotList, tt.wantList) {
				t.Errorf("AuthorizationStrategyAllowAll.AllowedCloudServices() got1 = %v, want %v", gotList, tt.wantList)
			}
		})
	}
}

func TestAuthorizationStrategyJWT_CheckAccess(t *testing.T) {
	type fields struct {
		Key string
	}
	type args struct {
		ctx context.Context
		typ RequestType
		req orchestrator.CloudServiceRequest
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "valid context",
			fields: fields{
				Key: testutil.TestCustomClaims,
			},
			args: args{
				ctx: testutil.TestContextOnlyService1,
				typ: AccessRead,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: testutil.TestCloudService1},
			},
			want: true,
		},
		{
			name: "valid context, wrong claim",
			fields: fields{
				Key: "sub",
			},
			args: args{
				ctx: testutil.TestContextOnlyService1,
				typ: AccessRead,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: testutil.TestCloudService1},
			},
			want: false,
		},
		{
			name: "valid context, ignore non-string",
			fields: fields{
				Key: "other",
			},
			args: args{
				ctx: testutil.TestContextOnlyService1,
				typ: AccessRead,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: testutil.TestCloudService1},
			},
			want: false,
		},
		{
			name: "missing token",
			fields: fields{
				Key: testutil.TestCustomClaims,
			},
			args: args{
				ctx: context.Background(),
				typ: AccessRead,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: testutil.TestCloudService1},
			},
			want: false,
		},
		{
			name: "broken token",
			fields: fields{
				Key: testutil.TestCustomClaims,
			},
			args: args{
				ctx: testutil.TestBrokenContext,
				typ: AccessRead,
				req: &orchestrator.GetCloudServiceRequest{CloudServiceId: testutil.TestCloudService1},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AuthorizationStrategyJWT{
				Key: tt.fields.Key,
			}
			if got := a.CheckAccess(tt.args.ctx, tt.args.typ, tt.args.req); got != tt.want {
				t.Errorf("AuthorizationStrategyJWT.CheckAccess() = %v, want %v", got, tt.want)
			}
		})
	}
}