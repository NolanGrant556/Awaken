import { safeStorage } from "electron";

const SECURE_CREDENTIAL_SCHEME = "electron-safe-storage" as const;
const CURRENT_SECURE_CREDENTIAL_VERSION = 2 as const;
const TEMPORARILY_UNAVAILABLE_PATTERN = /temporarily unavailable/i;

export type SecureCredentialErrorCode =
  | "temporarily_unavailable"
  | "invalid_ciphertext"
  | "unsupported_version"
  | "encrypt_failed"
  | "decrypt_failed";

export class SecureCredentialError extends Error {
  constructor(
    readonly code: SecureCredentialErrorCode,
    message: string,
    readonly retryable = false,
  ) {
    super(message);
    this.name = "SecureCredentialError";
  }
}

/**
 * Version 1 credentials predate the explicit version/scheme fields. Keeping
 * encryptedApiKey stable lets older Awaken builds continue to read version 2
 * records because Electron's sync and async safeStorage ciphertext formats are
 * interoperable.
 */
export type StoredSecureCredential = {
  encryptedApiKey: string;
  last4: string;
  version?: 1 | 2;
  scheme?: typeof SECURE_CREDENTIAL_SCHEME;
};

export type DecryptedSecureCredential = {
  plaintext: string;
  replacement?: StoredSecureCredential;
};

export interface SecureCredentialStore {
  encrypt(plaintext: string): Promise<StoredSecureCredential>;
  decrypt(credential: StoredSecureCredential): Promise<DecryptedSecureCredential>;
}

export type AsyncSafeStorage = {
  encryptStringAsync(plaintext: string): Promise<Buffer>;
  decryptStringAsync(
    encrypted: Buffer,
  ): Promise<{ result: string; shouldReEncrypt: boolean }>;
};

type SecureCredentialStoreOptions = {
  storage?: AsyncSafeStorage;
};

function isTemporarilyUnavailable(error: unknown): boolean {
  return error instanceof Error && TEMPORARILY_UNAVAILABLE_PATTERN.test(error.message);
}

function strictBase64Decode(value: unknown): Buffer {
  if (typeof value !== "string" || value.length === 0) {
    throw new SecureCredentialError(
      "invalid_ciphertext",
      "已保存的安全凭据数据无效，请重新保存 API key。",
    );
  }

  const normalized = value.trim();
  if (
    normalized.length === 0 ||
    normalized.length % 4 !== 0 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(
      normalized,
    )
  ) {
    throw new SecureCredentialError(
      "invalid_ciphertext",
      "已保存的安全凭据数据无效，请重新保存 API key。",
    );
  }

  const decoded = Buffer.from(normalized, "base64");
  if (decoded.length === 0 || decoded.toString("base64") !== normalized) {
    throw new SecureCredentialError(
      "invalid_ciphertext",
      "已保存的安全凭据数据无效，请重新保存 API key。",
    );
  }
  return decoded;
}

export class ElectronSecureCredentialStore implements SecureCredentialStore {
  private readonly storage: AsyncSafeStorage;

  constructor(options: SecureCredentialStoreOptions = {}) {
    this.storage = options.storage ?? safeStorage;
  }

  async encrypt(plaintext: string): Promise<StoredSecureCredential> {
    const encrypted = await this.runSafeStorageOperation(
      () => this.storage.encryptStringAsync(plaintext),
      "encrypt_failed",
    );
    if (!Buffer.isBuffer(encrypted) || encrypted.length === 0) {
      throw new SecureCredentialError(
        "encrypt_failed",
        "无法安全保存 API key，请稍后重试。",
      );
    }
    return {
      version: CURRENT_SECURE_CREDENTIAL_VERSION,
      scheme: SECURE_CREDENTIAL_SCHEME,
      encryptedApiKey: encrypted.toString("base64"),
      last4: plaintext.slice(-4),
    };
  }

  async decrypt(
    credential: StoredSecureCredential,
  ): Promise<DecryptedSecureCredential> {
    this.assertSupportedEnvelope(credential);
    const encrypted = strictBase64Decode(credential.encryptedApiKey);

    const decrypted = await this.runSafeStorageOperation(
      () => this.storage.decryptStringAsync(encrypted),
      "decrypt_failed",
    );
    if (
      !decrypted ||
      typeof decrypted.result !== "string" ||
      typeof decrypted.shouldReEncrypt !== "boolean"
    ) {
      throw new SecureCredentialError(
        "decrypt_failed",
        "无法读取已保存的 API key，请重新保存该凭据。",
      );
    }

    const needsReplacement =
      credential.version !== CURRENT_SECURE_CREDENTIAL_VERSION ||
      credential.scheme !== SECURE_CREDENTIAL_SCHEME ||
      decrypted.shouldReEncrypt;
    if (!needsReplacement) {
      return { plaintext: decrypted.result };
    }

    // Re-encryption is maintenance after a successful read. A transient
    // failure must not discard the usable plaintext or the original blob.
    try {
      const replacement = await this.encrypt(decrypted.result);
      return { plaintext: decrypted.result, replacement };
    } catch {
      return { plaintext: decrypted.result };
    }
  }

  private assertSupportedEnvelope(credential: StoredSecureCredential): void {
    if (
      credential.version !== undefined &&
      credential.version !== 1 &&
      credential.version !== CURRENT_SECURE_CREDENTIAL_VERSION
    ) {
      throw new SecureCredentialError(
        "unsupported_version",
        "已保存的安全凭据版本不受支持，请升级 Awaken 或重新保存 API key。",
      );
    }
    if (
      credential.version === CURRENT_SECURE_CREDENTIAL_VERSION &&
      credential.scheme !== SECURE_CREDENTIAL_SCHEME
    ) {
      throw new SecureCredentialError(
        "unsupported_version",
        "已保存的安全凭据格式不受支持，请升级 Awaken 或重新保存 API key。",
      );
    }
  }

  private async runSafeStorageOperation<T>(
    operation: () => Promise<T>,
    failureCode: "encrypt_failed" | "decrypt_failed",
  ): Promise<T> {
    try {
      return await operation();
    } catch (error) {
      if (isTemporarilyUnavailable(error)) {
        // Electron caches a temporarily unavailable provider key for the
        // process lifetime. Relaunch is the retry boundary; another call in
        // this process would deterministically fail with the same state.
        throw new SecureCredentialError(
          "temporarily_unavailable",
          "安全凭据服务暂时不可用，请重新启动 Awaken 后重试。",
          true,
        );
      }
      throw this.operationFailure(failureCode);
    }
  }

  private operationFailure(
    code: "encrypt_failed" | "decrypt_failed",
  ): SecureCredentialError {
    return code === "encrypt_failed"
      ? new SecureCredentialError(code, "无法安全保存 API key，请稍后重试。")
      : new SecureCredentialError(code, "无法读取已保存的 API key，请重新保存该凭据。");
  }
}
