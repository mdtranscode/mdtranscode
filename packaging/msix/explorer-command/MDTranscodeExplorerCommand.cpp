#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl_core.h>

#include <cstring>
#include <cwchar>
#include <new>
#include <string>
#include <vector>

namespace {

// Stable CLSID for the packaged MDTranscode Windows 11 Explorer command.
// This CLSID is mirrored in the private AppxManifest template.
const CLSID CLSID_MDTranscodeExplorerCommand =
    {0xa1672651, 0x3944, 0x46cf, {0xa1, 0x38, 0xb4, 0xe1, 0x21, 0x5f, 0x52, 0x80}};

HMODULE g_module = nullptr;
volatile LONG g_objectCount = 0;
volatile LONG g_serverLocks = 0;

HRESULT CopyToCoTaskMem(const std::wstring& value, PWSTR* result) {
    if (!result) {
        return E_POINTER;
    }

    *result = nullptr;
    const size_t bytes = (value.size() + 1) * sizeof(wchar_t);
    auto* copy = static_cast<PWSTR>(CoTaskMemAlloc(bytes));
    if (!copy) {
        return E_OUTOFMEMORY;
    }

    memcpy(copy, value.c_str(), bytes);
    *result = copy;
    return S_OK;
}

std::wstring ModuleDirectory() {
    std::vector<wchar_t> buffer(32768, L'\0');
    const DWORD length = GetModuleFileNameW(g_module, buffer.data(), static_cast<DWORD>(buffer.size()));
    if (length == 0 || length >= buffer.size()) {
        return {};
    }

    std::wstring path(buffer.data(), length);
    const size_t slash = path.find_last_of(L"\\/");
    if (slash == std::wstring::npos) {
        return {};
    }

    return path.substr(0, slash);
}

std::wstring ApplicationPath() {
    const std::wstring directory = ModuleDirectory();
    if (directory.empty()) {
        return {};
    }
    return directory + L"\\MDTranscode.exe";
}

bool IsMarkdownPath(const std::wstring& path) {
    const size_t dot = path.find_last_of(L'.');
    if (dot == std::wstring::npos) {
        return false;
    }

    const std::wstring extension = path.substr(dot);
    return _wcsicmp(extension.c_str(), L".md") == 0 ||
           _wcsicmp(extension.c_str(), L".markdown") == 0;
}

HRESULT GetSingleSelectedPath(IShellItemArray* items, std::wstring* path) {
    if (!items || !path) {
        return E_POINTER;
    }

    DWORD count = 0;
    HRESULT hr = items->GetCount(&count);
    if (FAILED(hr)) {
        return hr;
    }
    if (count != 1) {
        return E_INVALIDARG;
    }

    IShellItem* item = nullptr;
    hr = items->GetItemAt(0, &item);
    if (FAILED(hr)) {
        return hr;
    }

    PWSTR rawPath = nullptr;
    hr = item->GetDisplayName(SIGDN_FILESYSPATH, &rawPath);
    item->Release();
    if (FAILED(hr)) {
        return hr;
    }

    *path = rawPath;
    CoTaskMemFree(rawPath);
    return S_OK;
}

class ExplorerCommand final : public IExplorerCommand {
public:
    ExplorerCommand() : refCount_(1) {
        InterlockedIncrement(&g_objectCount);
    }

    IFACEMETHODIMP QueryInterface(REFIID iid, void** object) override {
        if (!object) {
            return E_POINTER;
        }

        *object = nullptr;
        if (IsEqualIID(iid, IID_IUnknown) || IsEqualIID(iid, IID_IExplorerCommand)) {
            *object = static_cast<IExplorerCommand*>(this);
            AddRef();
            return S_OK;
        }

        return E_NOINTERFACE;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override {
        return static_cast<ULONG>(InterlockedIncrement(&refCount_));
    }

    IFACEMETHODIMP_(ULONG) Release() override {
        const LONG count = InterlockedDecrement(&refCount_);
        if (count == 0) {
            delete this;
        }
        return static_cast<ULONG>(count);
    }

    IFACEMETHODIMP GetTitle(IShellItemArray*, PWSTR* title) override {
        return CopyToCoTaskMem(L"Convert to Word with MDTranscode", title);
    }

    IFACEMETHODIMP GetIcon(IShellItemArray*, PWSTR* icon) override {
        const std::wstring executable = ApplicationPath();
        if (executable.empty()) {
            if (icon) {
                *icon = nullptr;
            }
            return E_NOTIMPL;
        }
        return CopyToCoTaskMem(executable + L",0", icon);
    }

    IFACEMETHODIMP GetToolTip(IShellItemArray*, PWSTR* tooltip) override {
        return CopyToCoTaskMem(L"Create a Word DOCX beside this Markdown file.", tooltip);
    }

    IFACEMETHODIMP GetCanonicalName(GUID* commandName) override {
        if (!commandName) {
            return E_POINTER;
        }
        *commandName = CLSID_MDTranscodeExplorerCommand;
        return S_OK;
    }

    IFACEMETHODIMP GetState(IShellItemArray* items, BOOL, EXPCMDSTATE* state) override {
        if (!state) {
            return E_POINTER;
        }

        *state = ECS_HIDDEN;
        std::wstring path;
        const HRESULT hr = GetSingleSelectedPath(items, &path);
        if (SUCCEEDED(hr) && IsMarkdownPath(path)) {
            *state = ECS_ENABLED;
        }
        return S_OK;
    }

    IFACEMETHODIMP Invoke(IShellItemArray* items, IBindCtx*) override {
        std::wstring sourcePath;
        HRESULT hr = GetSingleSelectedPath(items, &sourcePath);
        if (FAILED(hr)) {
            return hr;
        }
        if (!IsMarkdownPath(sourcePath)) {
            return HRESULT_FROM_WIN32(ERROR_NOT_SUPPORTED);
        }

        const std::wstring executable = ApplicationPath();
        const std::wstring directory = ModuleDirectory();
        if (executable.empty() || directory.empty()) {
            return HRESULT_FROM_WIN32(ERROR_PATH_NOT_FOUND);
        }
        if (GetFileAttributesW(executable.c_str()) == INVALID_FILE_ATTRIBUTES) {
            return HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND);
        }

        // Windows filenames cannot contain a double quote, so quoting the file
        // path is sufficient for the existing --convert-to-docx command parser.
        std::wstring commandLine = L"\"" + executable + L"\" --convert-to-docx \"" + sourcePath + L"\"";
        std::vector<wchar_t> mutableCommandLine(commandLine.begin(), commandLine.end());
        mutableCommandLine.push_back(L'\0');

        STARTUPINFOW startup{};
        startup.cb = sizeof(startup);
        PROCESS_INFORMATION process{};

        const BOOL created = CreateProcessW(
            executable.c_str(),
            mutableCommandLine.data(),
            nullptr,
            nullptr,
            FALSE,
            CREATE_UNICODE_ENVIRONMENT,
            nullptr,
            directory.c_str(),
            &startup,
            &process);

        if (!created) {
            return HRESULT_FROM_WIN32(GetLastError());
        }

        CloseHandle(process.hThread);
        CloseHandle(process.hProcess);
        return S_OK;
    }

    IFACEMETHODIMP GetFlags(EXPCMDFLAGS* flags) override {
        if (!flags) {
            return E_POINTER;
        }
        *flags = ECF_DEFAULT;
        return S_OK;
    }

    IFACEMETHODIMP EnumSubCommands(IEnumExplorerCommand** commands) override {
        if (!commands) {
            return E_POINTER;
        }
        *commands = nullptr;
        return E_NOTIMPL;
    }

private:
    ~ExplorerCommand() {
        InterlockedDecrement(&g_objectCount);
    }

    volatile LONG refCount_;
};

class ClassFactory final : public IClassFactory {
public:
    ClassFactory() : refCount_(1) {
        InterlockedIncrement(&g_objectCount);
    }

    IFACEMETHODIMP QueryInterface(REFIID iid, void** object) override {
        if (!object) {
            return E_POINTER;
        }

        *object = nullptr;
        if (IsEqualIID(iid, IID_IUnknown) || IsEqualIID(iid, IID_IClassFactory)) {
            *object = static_cast<IClassFactory*>(this);
            AddRef();
            return S_OK;
        }

        return E_NOINTERFACE;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override {
        return static_cast<ULONG>(InterlockedIncrement(&refCount_));
    }

    IFACEMETHODIMP_(ULONG) Release() override {
        const LONG count = InterlockedDecrement(&refCount_);
        if (count == 0) {
            delete this;
        }
        return static_cast<ULONG>(count);
    }

    IFACEMETHODIMP CreateInstance(IUnknown* outer, REFIID iid, void** object) override {
        if (outer) {
            return CLASS_E_NOAGGREGATION;
        }
        if (!object) {
            return E_POINTER;
        }

        *object = nullptr;
        auto* command = new (std::nothrow) ExplorerCommand();
        if (!command) {
            return E_OUTOFMEMORY;
        }

        const HRESULT hr = command->QueryInterface(iid, object);
        command->Release();
        return hr;
    }

    IFACEMETHODIMP LockServer(BOOL lock) override {
        if (lock) {
            InterlockedIncrement(&g_serverLocks);
        } else {
            InterlockedDecrement(&g_serverLocks);
        }
        return S_OK;
    }

private:
    ~ClassFactory() {
        InterlockedDecrement(&g_objectCount);
    }
    volatile LONG refCount_;
};

} // namespace

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        g_module = instance;
        DisableThreadLibraryCalls(instance);
    }
    return TRUE;
}

extern "C" HRESULT __stdcall DllGetClassObject(REFCLSID clsid, REFIID iid, void** object) {
    if (!IsEqualCLSID(clsid, CLSID_MDTranscodeExplorerCommand)) {
        return CLASS_E_CLASSNOTAVAILABLE;
    }
    if (!object) {
        return E_POINTER;
    }

    *object = nullptr;
    auto* factory = new (std::nothrow) ClassFactory();
    if (!factory) {
        return E_OUTOFMEMORY;
    }

    const HRESULT hr = factory->QueryInterface(iid, object);
    factory->Release();
    return hr;
}

extern "C" HRESULT __stdcall DllCanUnloadNow() {
    return (g_objectCount == 0 && g_serverLocks == 0) ? S_OK : S_FALSE;
}
