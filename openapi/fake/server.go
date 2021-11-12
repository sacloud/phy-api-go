// Copyright 2021 The phy-go authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fake

import (
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/sacloud/phy-go/openapi"
)

type Server struct{}

func (s *Server) Start() *httptest.Server {
	router := gin.Default()

	router.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})
	return httptest.NewServer(openapi.RegisterHandlers(router, s))
}