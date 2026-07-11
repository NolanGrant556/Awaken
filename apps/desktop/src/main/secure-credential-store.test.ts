import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const electronMock = vi.hoisted(() => ({
  safeStorage: {
    decryptString: vi.fn(),
    decryptStringAsync: vi.fn(),
    encryptString: vi.fn(),
    encryptStringAsync: vi.fn(),
    isAsyncEncryptionAvailable: vi.fn(),
    isEncryptionAvailable: vi.fn(),
    setUsePlainTextEncryption: vi.fn(),
  },
}));

vi.mock("electron", () => electronMock);

import {
  ElectronSecureCredentialStore,
  type AsyncSafeStorage,
  type StoredSecureCredential,
} from "./secure-credential-store.js";

const ciphertext = Buffer.from("v10-encrypted-value");

function storedCredential(
  overrides: Partial<StoredSecureCredential> = {},
): StoredSecureCredential {
  return {
    version: 2,
    scheme: "electron-safe-storage",
    encryptedApiKey: ciphertext.toString("base64"),
    last4: "cret",
    ...overrides,
  };
}

function fakeStorage(): AsyncSafeStorage {
  return {
    encryptStringAsync: vi.fn(async () => ciphertext),
    decryptStringAsync: vi.fn(async () => ({
      result: "test-secret",
      shouldReEncrypt: false,
    })),
  };
}

beforeEach(() => {
  electronMock.safeStorage.encryptStringAsync.mockResolvedValue(ciphertext);
  electronMock.safeStorage.decryptStringAsync.mockResolvedValue({
    result: "test-secret",
    shouldReEncrypt: false,
  });
});
afterEach(() => {
  vi.clearAllMocks();
});

describe("ElectronSecureCredentialStore", () => {
  it("uses only async safeStorage and writes a rollback-compatible v2 envelope", async () => {
    const store = new ElectronSecureCredentialStore();

    const result = await store.encrypt("test-secret");

    expect(result).toEqual({
      version: 2,
      scheme: "electron-safe-storage",
      encryptedApiKey: ciphertext.toString("base64"),
      last4: "cret",
    });
    expect(electronMock.safeStorage.encryptStringAsync).toHaveBeenCalledWith("test-secret");
    expect(electronMock.safeStorage.isAsyncEncryptionAvailable).not.toHaveBeenCalled();
    expect(electronMock.safeStorage.isEncryptionAvailable).not.toHaveBeenCalled();
    expect(electronMock.safeStorage.encryptString).not.toHaveBeenCalled();
    expect(electronMock.safeStorage.decryptString).not.toHaveBeenCalled();
    expect(electronMock.safeStorage.setUsePlainTextEncryption).not.toHaveBeenCalled();
  });

  it("decrypts a v2 envelope without rewriting it", async () => {
    const storage = fakeStorage();
    const store = new ElectronSecureCredentialStore({ storage });

    const result = await store.decrypt(storedCredential());

    expect(result).toEqual({ plaintext: "test-secret" });
    expect(storage.decryptStringAsync).toHaveBeenCalledWith(ciphertext);
    expect(storage.encryptStringAsync).not.toHaveBeenCalled();
  });

  it("classifies temporary provider failure without a futile same-process retry", async () => {
    const storage = fakeStorage();
    vi.mocked(storage.decryptStringAsync).mockRejectedValue(
      new Error(
        "safeStorage.decryptStringAsync is temporarily unavailable. Please try again.",
      ),
    );
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(store.decrypt(storedCredential())).rejects.toMatchObject({
      name: "SecureCredentialError",
      code: "temporarily_unavailable",
      retryable: true,
      message: "安全凭据服务暂时不可用，请重新启动 Awaken 后重试。",
    });
    expect(storage.decryptStringAsync).toHaveBeenCalledTimes(1);
  });

  it("classifies permanent decrypt failure without retrying", async () => {
    const storage = fakeStorage();
    vi.mocked(storage.decryptStringAsync).mockRejectedValue(
      new Error("Error while decrypting the ciphertext provided to safeStorage.decryptStringAsync."),
    );
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(store.decrypt(storedCredential())).rejects.toMatchObject({
      code: "decrypt_failed",
      retryable: false,
    });
    expect(storage.decryptStringAsync).toHaveBeenCalledTimes(1);
  });

  it("lazily migrates an unversioned legacy envelope after successful decrypt", async () => {
    const storage = fakeStorage();
    const replacementCiphertext = Buffer.from("v10-replacement");
    vi.mocked(storage.encryptStringAsync).mockResolvedValue(replacementCiphertext);
    const store = new ElectronSecureCredentialStore({ storage });
    const legacy = storedCredential({ version: undefined, scheme: undefined });

    const result = await store.decrypt(legacy);

    expect(result).toEqual({
      plaintext: "test-secret",
      replacement: {
        version: 2,
        scheme: "electron-safe-storage",
        encryptedApiKey: replacementCiphertext.toString("base64"),
        last4: "cret",
      },
    });
  });

  it("honors Electron key-rotation requests", async () => {
    const storage = fakeStorage();
    vi.mocked(storage.decryptStringAsync).mockResolvedValue({
      result: "test-secret",
      shouldReEncrypt: true,
    });
    const store = new ElectronSecureCredentialStore({ storage });

    const result = await store.decrypt(storedCredential());

    expect(result.replacement).toMatchObject({
      version: 2,
      scheme: "electron-safe-storage",
    });
    expect(storage.encryptStringAsync).toHaveBeenCalledWith("test-secret");
  });

  it("keeps the usable plaintext and original envelope when maintenance re-encryption fails", async () => {
    const storage = fakeStorage();
    vi.mocked(storage.encryptStringAsync).mockRejectedValue(new Error("encrypt failed"));
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(
      store.decrypt(storedCredential({ version: undefined, scheme: undefined })),
    ).resolves.toEqual({ plaintext: "test-secret" });
  });

  it("rejects malformed base64 before calling safeStorage", async () => {
    const storage = fakeStorage();
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(
      store.decrypt(storedCredential({ encryptedApiKey: "not-base64" })),
    ).rejects.toMatchObject({ code: "invalid_ciphertext", retryable: false });
    expect(storage.decryptStringAsync).not.toHaveBeenCalled();
  });

  it("rejects unsupported envelope versions before calling safeStorage", async () => {
    const storage = fakeStorage();
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(
      store.decrypt(storedCredential({ version: 3 as 2 })),
    ).rejects.toMatchObject({ code: "unsupported_version", retryable: false });
    expect(storage.decryptStringAsync).not.toHaveBeenCalled();
  });

  it("classifies encryption failures without writing a plaintext fallback", async () => {
    const storage = fakeStorage();
    vi.mocked(storage.encryptStringAsync).mockRejectedValue(new Error("encrypt failed"));
    const store = new ElectronSecureCredentialStore({ storage });

    await expect(store.encrypt("test-secret")).rejects.toMatchObject({
      code: "encrypt_failed",
      retryable: false,
    });
  });
});
