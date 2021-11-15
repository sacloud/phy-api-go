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
	"fmt"
	"time"

	"github.com/getlantern/deepcopy"
	"github.com/sacloud/phy-go/openapi"
)

// ListServers サーバー一覧
// (GET /servers/)
func (engine *Engine) ListServers(params openapi.ListServersParams) (*openapi.Servers, error) {
	defer engine.rLock()()

	// TODO 検索条件の処理を実装

	return &openapi.Servers{
		Meta: openapi.PaginateMeta{
			Count: len(engine.Servers),
		},
		Servers: engine.servers(),
	}, nil
}

// ReadServer サーバー
// (GET /servers/{server_id}/)
func (engine *Engine) ReadServer(serverId openapi.ServerId) (*openapi.Server, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		var server openapi.Server
		if err := deepcopy.Copy(&server, s.Server); err != nil {
			return nil, err
		}
		return &server, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", string(serverId))
}

// ListOSImages インストール可能OS一覧
// (GET /servers/{server_id}/os_images/)
func (engine *Engine) ListOSImages(serverId openapi.ServerId) ([]*openapi.OsImage, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		var images []*openapi.OsImage
		if err := deepcopy.Copy(&images, s.OSImages); err != nil {
			return nil, err
		}
		return images, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", string(serverId))
}

// OSInstall OSインストールの実行
// (POST /servers/{server_id}/os_install/)
func (engine *Engine) OSInstall(serverId openapi.ServerId, params openapi.OsInstallParameter) error {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		if s.Server.LockStatus != nil {
			return NewError(ErrorTypeConflict, "server", string(serverId))
		}
		for _, image := range s.OSImages {
			if image.OsImageId == params.OsImageId {
				engine.startOSInstall(s)
				return nil
			}
		}
		return NewError(ErrorTypeNotFound, "os-image", params.OsImageId, "server[%s]", serverId)
	}
	return NewError(ErrorTypeNotFound, "server", serverId)
}

// ReadServerPortChannel ポートチャネル状態取得
// (GET /servers/{server_id}/port_channels/{port_channel_id}/)
func (engine *Engine) ReadServerPortChannel(serverId openapi.ServerId, portChannelId openapi.PortChannelId) (*openapi.PortChannel, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		return s.getPortChannelById(portChannelId)
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ServerConfigureBonding ポートチャネル ボンディング設定
// (POST /servers/{server_id}/port_channels/{port_channel_id}/configure_bonding/)
//
// この実装では排他ロックをかけて同期的に処理するため対象ポートチャネルのLockedは更新しない
func (engine *Engine) ServerConfigureBonding(serverId openapi.ServerId, portChannelId openapi.PortChannelId, params openapi.ConfigureBondingParameter) (*openapi.PortChannel, error) {
	defer engine.lock()() // ここで同期的に更新処理を行うため書き込みロック

	s := engine.getServerById(serverId)
	if s != nil {
		portChannel, err := s.getPortChannelById(portChannelId)
		if err != nil {
			return nil, err
		}
		// BondingTypeとserver.spec.{port_channel_1gbe_count, port_channel_10gbe_count}に応じて
		// 必要な数だけportを作成
		var ports []openapi.InterfacePort
		var portIds []int

		switch params.BondingType {
		case openapi.BondingTypeLacp, openapi.BondingTypeStatic:
			if params.PortNicknames != nil && len(*params.PortNicknames) != 1 {
				return nil, NewError(ErrorTypeInvalidRequest, "port-channel", portChannelId, "invalid PortNicknames")
			}
			name := string(portChannel.LinkSpeedType)
			if params.PortNicknames != nil {
				names := *params.PortNicknames
				if names[0] != "" {
					name = names[0]
				}
			}
			port := openapi.InterfacePort{
				Enabled:       true,
				Nickname:      name,
				PortChannelId: portChannel.PortChannelId,
				PortId:        engine.nextId(),
			}

			ports = append(ports, port)
			portIds = append(portIds, port.PortId)
		case openapi.BondingTypeSingle:
			if params.PortNicknames != nil && len(*params.PortNicknames) != 2 {
				return nil, NewError(ErrorTypeInvalidRequest, "port-channel", portChannelId, "invalid PortNicknames")
			}

			prefix := string(portChannel.LinkSpeedType)
			names := []string{prefix + " 1", prefix + " 2"}
			if params.PortNicknames != nil {
				names = *params.PortNicknames
			}
			for _, name := range names {
				port := openapi.InterfacePort{
					Enabled:       true,
					Nickname:      name,
					PortChannelId: portChannel.PortChannelId,
					PortId:        engine.nextId(),
				}
				ports = append(ports, port)
				portIds = append(portIds, port.PortId)
			}
		}
		s.Server.Ports = ports
		portChannel.Ports = portIds

		s.updatePortChannel(portChannel)
		return portChannel, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ReadServerPort ポート情報取得
// (GET /servers/{server_id}/ports/{port_id}/)
func (engine *Engine) ReadServerPort(serverId openapi.ServerId, portId openapi.PortId) (*openapi.InterfacePort, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		return s.getPortById(portId)
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// UpdateServerPort ポート名称設定
// (PATCH /servers/{server_id}/ports/{port_id}/)
func (engine *Engine) UpdateServerPort(serverId openapi.ServerId, portId openapi.PortId, params openapi.UpdateServerPortParameter) (*openapi.InterfacePort, error) {
	defer engine.lock()()

	s := engine.getServerById(serverId)
	if s != nil {
		port, err := s.getPortById(portId)
		if err != nil {
			return nil, err
		}
		port.Nickname = params.Nickname

		s.updatePort(port)
		return port, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ServerAssignNetwork ネットワーク接続設定の変更
// (POST /servers/{server_id}/ports/{port_id}/assign_network/)
//
// Note: この実装では本来不可能な複数のインターネット接続(がされたポート)を許容している。
// 必要に応じて利用者側で適切にハンドリングすること。
func (engine *Engine) ServerAssignNetwork(serverId openapi.ServerId, portId openapi.PortId, params openapi.AssignNetworkParameter) (*openapi.InterfacePort, error) {
	defer engine.lock()()

	s := engine.getServerById(serverId)
	if s != nil {
		port, err := s.getPortById(portId)
		if err != nil {
			return nil, err
		}

		// 一旦関連する項目をリセット
		port.Internet = nil
		port.Mode = nil
		port.PrivateNetworks = nil
		port.GlobalBandwidthMbps = nil
		port.LocalBandwidthMbps = nil

		var internet *openapi.Internet
		if params.InternetType != nil {
			switch *params.InternetType {
			case openapi.AssignNetworkParameterInternetTypeCommonSubnet:
				// TODO 共用グローバルネットをどこかに定義しておく
				internet = &openapi.Internet{
					NetworkAddress: "203.0.113.0",
					PrefixLength:   24,
					SubnetType:     openapi.InternetSubnetTypeCommonSubnet,
				}
				mbps := 100
				port.GlobalBandwidthMbps = &mbps
			case openapi.AssignNetworkParameterInternetTypeDedicatedSubnet:
				subnet := engine.getDedicatedSubnetById(openapi.DedicatedSubnetId(*params.DedicatedSubnetId))
				if subnet == nil {
					return nil, NewError(ErrorTypeInvalidRequest, "port", portId, "invalid dedicated subnet id: %s", params.DedicatedSubnetId)
				}
				internet = &openapi.Internet{
					DedicatedSubnet: &openapi.AttachedDedicatedSubnet{
						DedicatedSubnetId: subnet.DedicatedSubnetId,
						Nickname:          subnet.Service.Nickname,
					},
					NetworkAddress: subnet.Ipv4.NetworkAddress,
					PrefixLength:   subnet.Ipv4.PrefixLength,
					SubnetType:     openapi.InternetSubnetTypeDedicatedSubnet,
				}
				mbps := 500
				port.GlobalBandwidthMbps = &mbps
			default:
				panic(fmt.Errorf("invalid InternetType: %v", params.InternetType))
			}
		}
		port.Internet = internet

		switch params.Mode {
		case openapi.AssignNetworkParameterModeAccess:
			v := openapi.InterfacePortModeAccess
			port.Mode = &v
		case openapi.AssignNetworkParameterModeTrunk:
			v := openapi.InterfacePortModeTrunk
			port.Mode = &v
		}

		if params.PrivateNetworkIds != nil {
			for _, id := range *params.PrivateNetworkIds {
				pn := engine.getPrivateNetworkById(openapi.PrivateNetworkId(id))
				if pn == nil {
					return nil, NewError(ErrorTypeInvalidRequest, "port", portId, "invalid private network id: %s", id)
				}
				port.PrivateNetworks = append(port.PrivateNetworks, openapi.AttachedPrivateNetwork{
					Nickname:         pn.Service.Nickname,
					PrivateNetworkId: pn.PrivateNetworkId,
				})
			}
			// 2000もあり得るがこの実装では1000で固定
			mbps := 1000
			port.LocalBandwidthMbps = &mbps
		}

		s.updatePort(port)
		return port, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// EnableServerPort ポート有効/無効設定
// (POST /servers/{server_id}/ports/{port_id}/enable/)
func (engine *Engine) EnableServerPort(serverId openapi.ServerId, portId openapi.PortId, params openapi.EnableServerPortParameter) (*openapi.InterfacePort, error) {
	defer engine.lock()()

	s := engine.getServerById(serverId)
	if s != nil {
		port, err := s.getPortById(portId)
		if err != nil {
			return nil, err
		}
		port.Enabled = params.Enable

		s.updatePort(port)
		return port, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ReadServerTrafficByPort トラフィックデータ取得
// (GET /servers/{server_id}/ports/{port_id}/traffic_graph/)
//
// Note: この実装では対象サーバが存在する場合は固定のレスポンスを返すのみ
func (engine *Engine) ReadServerTrafficByPort(serverId openapi.ServerId, portId openapi.PortId, params openapi.ReadServerTrafficByPortParams) (*openapi.TrafficGraph, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		return &openapi.TrafficGraph{
			Receive: []openapi.TrafficGraphData{
				{
					Timestamp: time.Now(),
					Value:     1,
				},
				{
					Timestamp: time.Now().Add(-1 * time.Minute),
					Value:     2,
				},
			},
			Transmit: []openapi.TrafficGraphData{
				{
					Timestamp: time.Now(),
					Value:     1,
				},
				{
					Timestamp: time.Now().Add(-1 * time.Minute),
					Value:     2,
				},
			},
		}, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ServerPowerControl サーバーの電源操作
// (POST /servers/{server_id}/power_control/)
func (engine *Engine) ServerPowerControl(serverId openapi.ServerId, params openapi.PowerControlParameter) error {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		if s.Server.LockStatus != nil {
			return NewError(ErrorTypeConflict, "server", string(serverId))
		}

		engine.startServerPowerControl(s, params)
		return nil
	}
	return NewError(ErrorTypeNotFound, "server", serverId)
}

// ReadServerPowerStatus サーバーの電源情報を取得する
// (GET /servers/{server_id}/power_status/)
func (engine *Engine) ReadServerPowerStatus(serverId openapi.ServerId) (*openapi.ServerPowerStatus, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		return s.PowerStatus, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// ReadRAIDStatus サーバーのRAID状態を取得
// (GET /servers/{server_id}/raid_status/)
//
// Note: この実装ではrefreshパラメータは無視される
func (engine *Engine) ReadRAIDStatus(serverId openapi.ServerId, params openapi.ReadRAIDStatusParams) (*openapi.RaidStatus, error) {
	defer engine.rLock()()

	s := engine.getServerById(serverId)
	if s != nil {
		return s.RaidStatus, nil
	}
	return nil, NewError(ErrorTypeNotFound, "server", serverId)
}

// servers []*ServerDataから[]openapi.Serverに変換して返す
func (engine *Engine) servers() []openapi.Server {
	var results []openapi.Server
	for _, s := range engine.Servers {
		results = append(results, *s.Server)
	}
	return results
}

func (engine *Engine) getServerById(serverId openapi.ServerId) *Server {
	for _, s := range engine.Servers {
		if s.Id() == string(serverId) {
			return s
		}
	}
	return nil
}

func (engine *Engine) startOSInstall(server *Server) {
	go engine.startUpdateAction(func() {
		//start
		status := openapi.ServerLockStatusOsInstall
		server.Server.LockStatus = &status

		// finish
		go engine.startUpdateAction(func() {
			server.Server.LockStatus = nil
		})
	})
}

func (engine *Engine) startServerPowerControl(server *Server, params openapi.PowerControlParameter) {
	var powerStates openapi.ServerPowerStatusStatus
	var cachedPowerStatus openapi.CachedPowerStatusStatus
	switch string(params.Operation) {
	case "on", "reset":
		powerStates = openapi.ServerPowerStatusStatusOn
		cachedPowerStatus = openapi.CachedPowerStatusStatusOn
	case "soft", "off":
		powerStates = openapi.ServerPowerStatusStatusOff
		cachedPowerStatus = openapi.CachedPowerStatusStatusOff
	}

	go engine.startUpdateAction(func() {
		server.PowerStatus = &openapi.ServerPowerStatus{
			Status: powerStates,
		}
		server.Server.CachedPowerStatus = &openapi.CachedPowerStatus{
			Status: cachedPowerStatus,
			Stored: time.Now(),
		}
	})
}