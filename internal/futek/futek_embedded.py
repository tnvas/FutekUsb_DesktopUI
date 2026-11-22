# futek_embedded.py
# Single-file module designed to be executed via `python -c "<string>"`
# Reads one JSON object from stdin and writes one JSON object to stdout.
#
# Request shape:
#   {
#     "dll_path": "C:\\path\\to\\FUTEK_USB_DLL.dll",    # optional; or use env FUTEK_DLL
#     "serial_number": "631244",
#     "command": "get_rate" | "set_rate" | "get_calibration" | "get_adc_pga_setting",
#     "args": { ... }                                   # optional
#   }
#
# Response shape (success):
#   { "ok": true, "command": "...", "data": <any> }
#
# Response shape (error):
#   { "ok": false, "error": { "type": "...", "message": "...", "detail": "..."} }

import sys, json, os, re
import time # Added import for time.sleep

def _print_json(obj):
    sys.stdout.write(json.dumps(obj, separators=(",", ":"), ensure_ascii=False))
    sys.stdout.flush()

def _fail(tp, msg, detail=None):
    _print_json({"ok": False, "error": {"type": tp, "message": msg, "detail": detail}})
    sys.exit(0)

def _load_dll(dll_path):
    try:
        import clr  # pythonnet
    except Exception as e:
        _fail("ImportError", "pythonnet is required (import clr failed)", str(e))
    try:
        clr.AddReference(dll_path)
    except Exception as e:
        _fail("DLLLoadError", f"Failed to load FUTEK DLL at '{dll_path}'", str(e))
    try:
        from FUTEK_USB_DLL import USB_DLL
    except Exception as e:
        _fail("DLLSymbolError", "Could not import USB_DLL from FUTEK DLL", str(e))
    return USB_DLL

def _open_device(USB_DLL_cls, serial_number):
    dev = USB_DLL_cls()
    r = dev.Open_Device_Connection(str(serial_number))
    if r == "Error":
        _fail("DeviceOpenError", f"Open_Device_Connection failed for SN={serial_number}", getattr(dev, "ErrorDescription", None))
    return dev

# Accept "00".."0F" (hex), decimal strings, ints, or SPS like 2400
_ADC_CODE_BY_SPS = {
    5: 0x00, 10: 0x01, 15: 0x02, 20: 0x03, 25: 0x04, 30: 0x05,
    50: 0x06, 60: 0x07, 100: 0x08, 300: 0x09, 600: 0x0A, 960: 0x0B,
    1200: 0x0C, 1600: 0x0D, 2400: 0x0E, 4800: 0x0F
}

def _parse_rate_code(x):
    # Returns an int 0..15, or raises ValueError
    if isinstance(x, int):
        if 0 <= x <= 0x0F:
            return x
        # maybe SPS passed as int
        if x in _ADC_CODE_BY_SPS:
            return _ADC_CODE_BY_SPS[x]
        raise ValueError("Rate code int out of range (0..15) and not a recognized SPS")
    if not isinstance(x, str):
        raise ValueError("rate_code must be str|int")
    s = x.strip()
    # Hex like "0E" or "0x0E"
    if re.fullmatch(r"(?i)0x[0-9a-f]{1,2}", s) or re.fullmatch(r"(?i)[0-9a-f]{1,2}", s):
        try:
            v = int(s, 16)
            if 0 <= v <= 0x0F:
                return v
        except Exception:
            pass
    # Decimal code 0..15
    if s.isdigit():
        v = int(s, 10)
        if 0 <= v <= 0x0F:
            return v
        # maybe SPS in decimal
        if v in _ADC_CODE_BY_SPS:
            return _ADC_CODE_BY_SPS[v]
    raise ValueError("Unrecognized rate_code; use hex '00'..'0F', decimal 0..15, or SPS like 2400")

def _cmd_get_rate(handle, dev):
    rate = dev.Get_ADC_Sampling_Rate_Setting(handle)
    if rate == "Error":
        _fail("CommandError", "Get_ADC_Sampling_Rate_Setting failed", getattr(dev, "ErrorDescription", None))
    # Normalize to int if numeric
    try:
        rate_int = int(str(rate).strip(), 10)
    except Exception:
        rate_int = rate
    return {"rate_code": rate_int}

def _cmd_set_rate(handle, dev, args):
    if not isinstance(args, dict) or "rate_code" not in args:
        _fail("BadArgs", "set_rate requires args.rate_code")
    try:
        code = _parse_rate_code(args["rate_code"])
    except Exception as e:
        _fail("BadArgs", str(e)) 
    
    # 1. Attempt to set the rate
    r = dev.Set_ADC_Configuration2(handle, int(code),1)
    if r == "Error":
        _fail("CommandError", "Set_ADC_Configuration failed", getattr(dev, "ErrorDescription", None))

    # 2. Add a short delay to allow the device/DLL to process the configuration change.
    # This often resolves 'Packet Number Error' issues which are timing/sequence related.
    time.sleep(0.05) # Delay for 50 milliseconds 
    
    # 3. Read back to confirm
    rb = dev.Get_ADC_Sampling_Rate_Setting(handle)
    if rb == "Error":
        _fail("CommandError", "Get_ADC_Sampling_Rate_Setting (verify) failed", getattr(dev, "ErrorDescription", None))
    try:
        rb_int = int(str(rb).strip(), 10)
    except Exception:
        rb_int = rb
    return {"rate_code": rb_int}

import ctypes

def _cmd_get_calibration(handle, dev):  
    handle_str=repr(handle) 
    if not handle:
        _fail("Invalid handle")
        return
    if handle_str == "0":
        _fail("Invalid handle")
        return
    n = dev.Get_Number_of_Loading_Points(handle)
    if n == "Error":
        _fail("CommandError", "Get_Number_of_Loading_Points failed", getattr(dev, "ErrorDescription", None))
    try:
        N = int(str(n))
    except Exception:
        _fail("ParseError", f"Invalid loading point count: {n}")
    pts = []
    for i in range(N):
        adc = dev.Get_Loading_Point(handle, i)
        load = dev.Get_Load_of_Loading_Point(handle, i)
        if adc == "Error" or load == "Error":
            _fail("CommandError", f"Error at index {i}", getattr(dev, "ErrorDescription", None))
        try:
            pts.append([int(str(adc)), int(str(load))])
        except Exception:
            _fail("ParseError", f"Bad pair at index {i}: adc={adc} load={load}")
    return {"points": pts}


def _cmd_get_adc_pga_setting(handle, dev):
    """
    Retrieves the ADC and PGA setting from the device.
    Expected DLL return format is a string like "ADC=128, PGA=100".
    """ 
    setting_str = dev.Get_ADC_PGA_Setting(handle)
    if setting_str == "Error":
        _fail("CommandError", "Get_ADC_PGA_Setting failed", getattr(dev, "ErrorDescription", None))

    # Pattern to match and extract ADC and PGA values
    match = re.search(r"ADC=(\d+).*PGA=(\d+)", str(setting_str), re.IGNORECASE)
    
    if match:
        try:
            adc = int(match.group(1))
            pga = int(match.group(2))
            return {"adc_setting": adc, "pga_setting": pga}
        except Exception:
            # Handle cases where the matched groups are not valid integers
            _fail("ParseError", f"Failed to parse ADC/PGA values from: {setting_str}")
    else:
        # Handle cases where the expected string format is not found
        _fail("ParseError", f"Unrecognized ADC/PGA format: {setting_str}")


def run_command(dll_path, serial_number, command, args=None):
    USB_DLL = _load_dll(dll_path)
    dev = _open_device(USB_DLL, serial_number)
    
    handle = dev.DeviceHandle
    try:
        cmd = (command or "").strip().lower()
        if cmd == "get_rate":
            data = _cmd_get_rate(handle, dev)
        elif cmd == "set_rate":
            data = _cmd_set_rate(handle, dev, args or {})
        elif cmd == "get_calibration":
            data = _cmd_get_calibration(handle, dev)
        elif cmd == "get_adc_pga_setting":
            data = _cmd_get_adc_pga_setting(handle, dev)
        else:
            _fail("UnknownCommand", f"Unsupported command: {command}")
        _print_json({"ok": True, "command": cmd, "data": data})
    finally:
        # Always try to close gracefully
        try:
            dev.Reset_Board(handle)
            dev.Close_Device_Connection(handle) 
        except Exception as e:
            print(e)
            _fail("CloseError", "failed to close the DLL connection", str(e))
            pass  # best effort

def _main(): 
    try:
        req = json.loads(sys.stdin.read())
    except Exception as e:
        _fail("InputError", "Failed to parse JSON request from stdin", str(e))
    dll_path = req.get("dll_path") or os.environ.get("FUTEK_DLL")
    if not dll_path:
        _fail("BadArgs", "dll_path is required (or set env FUTEK_DLL)")
    run_command(
        dll_path=dll_path,
        serial_number=req.get("serial_number"),
        command=req.get("command"),
        args=req.get("args") or {}
    )

if __name__ == "__main__":
    _main()
     