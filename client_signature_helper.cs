using System;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text;
using Newtonsoft.Json;

namespace Framework
{
    public static class SignatureHelper
    {
        // 这个密钥应该与服务器端的DefaultSignatureConfig.SecretKey一致
        private const string SECRET_KEY = "sparkinfi-game-secret-key";
        
        /// <summary>
        /// 为排行榜记录生成签名
        /// </summary>
        /// <param name="leaderboardId">排行榜ID</param>
        /// <param name="score">分数</param>
        /// <param name="subscore">子分数</param>
        /// <param name="metadata">元数据（不包含签名字段）</param>
        /// <returns>签名字符串</returns>
        public static string GenerateLeaderboardSignature(string leaderboardId, long score, long subscore, string metadata)
        {
            var data = new Dictionary<string, object>
            {
                ["leaderboard_id"] = leaderboardId,
                ["score"] = score,
                ["subscore"] = subscore,
                ["metadata"] = metadata ?? ""
            };
            
            return GenerateSignature(data);
        }
        
        /// <summary>
        /// 为锦标赛记录生成签名
        /// </summary>
        /// <param name="tournamentId">锦标赛ID</param>
        /// <param name="score">分数</param>
        /// <param name="subscore">子分数</param>
        /// <param name="metadata">元数据（不包含签名字段）</param>
        /// <returns>签名字符串</returns>
        public static string GenerateTournamentSignature(string tournamentId, long score, long subscore, string metadata)
        {
            var data = new Dictionary<string, object>
            {
                ["tournament_id"] = tournamentId,
                ["score"] = score,
                ["subscore"] = subscore,
                ["metadata"] = metadata ?? "{}"
            };
            
            return GenerateSignature(data);
        }
        
        
        /// <summary>
        /// 为钱包操作生成签名（更新版本，包含用户ID）
        /// </summary>
        /// <param name="operationType">操作类型：gain(增加) 或 consume(消费)</param>
        /// <param name="coin">金币数量</param>
        /// <param name="gem">钻石数量</param>
        /// <param name="ad">广告券数量</param>
        /// <param name="reason">操作原因</param>
        /// <param name="userId">用户ID</param>
        /// <returns>签名字符串</returns>
        public static string GenerateWalletSignature(string operationType, long coin, long gem, long ad, string reason, string userId)
        {
            var data = new Dictionary<string, object>
            {
                ["operation_type"] = operationType,
                ["coin"] = coin,
                ["gem"] = gem,
                ["ad"] = ad,
                ["reason"] = reason ?? "",
                ["user_id"] = userId ?? "",
            };
        
            return GenerateSignature(data);
        }

        /// <summary>
        /// 为钱包操作生成签名（旧版本，不包含用户ID）
        /// </summary>
        /// <param name="operationType">操作类型：gain(增加) 或 consume(消费)</param>
        /// <param name="coin">金币数量</param>
        /// <param name="gem">钻石数量</param>
        /// <param name="ad">广告券数量</param>
        /// <param name="reason">操作原因</param>
        /// <returns>签名字符串</returns>
        public static string GenerateWalletSignatureLegacy(string operationType, long coin, long gem, long ad, string reason)
        {
            var data = new Dictionary<string, object>
            {
                ["operation_type"] = operationType,
                ["coin"] = coin,
                ["gem"] = gem,
                ["ad"] = ad,
                ["reason"] = reason ?? "",
            };
        
            return GenerateSignature(data);
        }
        
        /// <summary>
        /// 为storage操作生成签名
        /// </summary>
        /// <param name="collection">存储集合名称</param>
        /// <param name="key">存储键名</param>
        /// <param name="value">存储值（JSON字符串，不包含signature字段）</param>
        /// <param name="userId">用户ID</param>
        /// <returns>签名字符串</returns>
        public static string GenerateStorageSignature(string collection, string key, string value, string userId)
        {
            var data = new Dictionary<string, object>
            {
                ["collection"] = collection ?? "",
                ["key"] = key ?? "",
                ["value"] = value ?? "",
                ["user_id"] = userId ?? "",
            };
        
            return GenerateSignature(data);
        }

        /// <summary>
        /// 为storage操作准备带签名的value
        /// </summary>
        /// <param name="collection">存储集合名称</param>
        /// <param name="key">存储键名</param>
        /// <param name="valueData">存储数据（不包含signature字段）</param>
        /// <param name="userId">用户ID</param>
        /// <returns>包含签名的完整value JSON字符串</returns>
        public static string PrepareStorageValueWithSignature(string collection, string key, Dictionary<string, object> valueData, string userId)
        {
            // 将valueData转换为JSON字符串（不包含signature）
            var valueJson = JsonConvert.SerializeObject(valueData);
            
            // 生成签名
            var signature = GenerateStorageSignature(collection, key, valueJson, userId);
            
            // 将签名添加到valueData中
            var valueWithSignature = new Dictionary<string, object>(valueData)
            {
                ["signature"] = signature
            };
            
            // 返回包含签名的完整JSON
            return JsonConvert.SerializeObject(valueWithSignature);
        }
        
        /// <summary>
        /// 生成HMAC-SHA256签名
        /// </summary>
        /// <param name="data">要签名的数据</param>
        /// <returns>签名字符串</returns>
        private static string GenerateSignature(Dictionary<string, object> data)
        {
            // 按key排序
            var sortedKeys = new List<string>(data.Keys);
            sortedKeys.Sort();
            
            // 构建签名字符串
            var parts = new List<string>();
            foreach (var key in sortedKeys)
            {
                var value = data[key];
                string valueStr = value switch
                {
                    string s => s,
                    long l => l.ToString(),
                    int i => i.ToString(),
                    double d => d.ToString(),
                    float f => f.ToString(),
                    bool b => b.ToString().ToLower(),
                    _ => value?.ToString() ?? ""
                };
                
                parts.Add($"{key}={valueStr}");
            }
            
            var signString = string.Join("&", parts);
            
            // 使用HMAC-SHA256生成签名
            using (var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(SECRET_KEY)))
            {
                var hash = hmac.ComputeHash(Encoding.UTF8.GetBytes(signString));
                return BitConverter.ToString(hash).Replace("-", "").ToLower();
            }
        }
    }
} 