import os
import subprocess
import sys
import hashlib
import glob

def get_tosutil_path():
    """获取 tosutil.exe 的完整路径"""
    script_dir = os.path.dirname(os.path.abspath(__file__))
    tosutil_path = os.path.join(script_dir, "tosutil.exe")
    if not os.path.exists(tosutil_path):
        print(f"错误: 在 {tosutil_path} 未找到 tosutil.exe")
        sys.exit(1)
    return tosutil_path

def configure_tos():
    """配置 TOS 工具"""
    tosutil = get_tosutil_path()
    config_command = f'"{tosutil}" config -i {access_key} -k {secret_key} -e {endpoint} -re {region}'
    try:
        subprocess.run(config_command, shell=True, check=True)
        print("TOS 已成功配置")
    except subprocess.CalledProcessError as e:
        print(f"配置 TOS 失败: {e}")
        sys.exit(1)

def upload_folder_to_tos(local_path, tos_path):
    """上传整个文件夹到 TOS"""
    tosutil = get_tosutil_path()
    upload_command = f'"{tosutil}" cp {local_path} tos://{bucket}/{tos_path} -r'
    try:
        subprocess.run(upload_command, shell=True, check=True)
        print(f"文件夹 {local_path} 已上传至 {tos_path}")
    except subprocess.CalledProcessError as e:
        print(f"上传文件夹失败: {e}")
        sys.exit(1)

def compute_file_hash(file_path):
    """计算文件的SHA256哈希值"""
    sha256 = hashlib.sha256()
    try:
        with open(file_path, 'rb') as f:
            for chunk in iter(lambda: f.read(4096), b""):
                sha256.update(chunk)
        return sha256.hexdigest()
    except FileNotFoundError:
        print(f"错误: 找不到文件 {file_path} 以计算哈希值。")
        sys.exit(1)

# 配置 AccessKey 和 SecretKey
access_key = "AKLTOTlhNmJkMWUwMDMzNGY5ZDhjY2RlNzk5ZjYxZmEwOTg"
secret_key = "WkdNMFlXUTROalU1TWpNMk5ETTJPV0poWVRZeFlXSXpZamhqTmpaaVlqWQ=="
endpoint = "tos-cn-beijing.volces.com"
region = "cn-beijing"
bucket = "xjtf"

if __name__ == "__main__":
    # 检查命令行参数
    if len(sys.argv) < 4:
        print("用法: python pub_addressable.py <cdnpath> <version> <upload_dir>")
        sys.exit(1)

    cdnpath = sys.argv[1]
    version = sys.argv[2]
    upload_dir = sys.argv[3]
    
    # 检查上传目录是否存在
    if not os.path.exists(upload_dir):
        print(f"错误: 上传目录不存在: {upload_dir}")
        sys.exit(1)
        
    # 检查上传目录是否为空
    if not os.listdir(upload_dir):
        print(f"错误: 上传目录为空: {upload_dir}")
        sys.exit(1)
    
    # 构建目标路径
    target_path = f"douyin/{cdnpath}/{version}"
    
    try:
        # 配置 TOS
        configure_tos()
        
        # 上传指定目录到 TOS
        upload_folder_to_tos(upload_dir, target_path)
        
        print(f"Addressable 资源已成功上传到 CDN: {target_path}")
    except Exception as e:
        print(f"上传过程中出错: {e}")
        sys.exit(1) 