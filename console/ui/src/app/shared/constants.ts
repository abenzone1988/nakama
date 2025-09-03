// Copyright 2020 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

export interface GameItem {
  id: string;
  name: string;
  icon: string;
}

/**
 * 默认游戏道具列表
 * 用于通知系统中的道具选择和显示
 */
export const DEFAULT_GAME_ITEMS: GameItem[] = [
  {id: '10000', name: '金币', icon: 'GoldCoin'},
  {id: '10001', name: '钻石', icon: 'Gem'},
  {id: '10002', name: '体力', icon: 'Strength'},
  {id: '20000', name: '广告券', icon: 'Coupon'},
];
