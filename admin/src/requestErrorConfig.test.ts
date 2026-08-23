import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { errorConfig, installRequestFeedback } from './requestErrorConfig';

const feedback = {
  warning: vi.fn(),
  error: vi.fn(),
  notify: vi.fn(),
};

let uninstallFeedback: (() => void) | undefined;

describe('requestErrorConfig', () => {
  // biome-ignore lint/style/noNonNullAssertion: config handlers are always defined
  const errorThrower = errorConfig.errorConfig!.errorThrower!;
  // biome-ignore lint/style/noNonNullAssertion: config handlers are always defined
  const errorHandler = errorConfig.errorConfig!.errorHandler!;

  beforeEach(() => {
    vi.clearAllMocks();
    uninstallFeedback = installRequestFeedback(feedback);
  });

  afterEach(() => uninstallFeedback?.());

  describe('errorThrower', () => {
    it('should throw error when success is false', () => {
      const response = {
        success: false,
        data: null,
        errorCode: 400,
        errorMessage: 'Bad Request',
        showType: 2,
      };

      expect(() => {
        errorThrower(response);
      }).toThrow('Bad Request');
    });

    it('should not throw error when success is true', () => {
      const response = {
        success: true,
        data: { id: 1 },
      };

      expect(() => {
        errorThrower(response);
      }).not.toThrow();
    });

    it('should throw BizError with correct info', () => {
      const response = {
        success: false,
        data: { detail: 'more info' },
        errorCode: 403,
        errorMessage: 'Forbidden',
        showType: 3,
      };

      expect.assertions(5);
      try {
        errorThrower(response);
      } catch (error: any) {
        expect(error.name).toBe('BizError');
        expect(error.info.errorCode).toBe(403);
        expect(error.info.errorMessage).toBe('Forbidden');
        expect(error.info.showType).toBe(3);
        expect(error.info.data).toEqual({ detail: 'more info' });
      }
    });
  });

  describe('errorHandler', () => {
    it('should rethrow error when skipErrorHandler is true', () => {
      const error = new Error('Test error');
      const opts = { skipErrorHandler: true };

      expect(() => {
        errorHandler(error, opts);
      }).toThrow('Test error');
    });

    it('should handle SILENT showType', () => {
      const error: any = new Error('Silent error');
      error.name = 'BizError';
      error.info = {
        errorCode: 1001,
        errorMessage: 'Silent error',
        showType: 0,
      };

      errorHandler(error, {});

      expect(feedback.warning).not.toHaveBeenCalled();
      expect(feedback.error).not.toHaveBeenCalled();
      expect(feedback.notify).not.toHaveBeenCalled();
    });

    it('should handle WARN_MESSAGE showType', () => {
      const error: any = new Error('Warning');
      error.name = 'BizError';
      error.info = {
        errorCode: 1002,
        errorMessage: 'This is a warning',
        showType: 1,
      };

      errorHandler(error, {});

      expect(feedback.warning).toHaveBeenCalledWith('This is a warning');
    });

    it('should handle ERROR_MESSAGE showType', () => {
      const error: any = new Error('Error message');
      error.name = 'BizError';
      error.info = {
        errorCode: 1003,
        errorMessage: 'This is an error',
        showType: 2,
      };

      errorHandler(error, {});

      expect(feedback.error).toHaveBeenCalledWith('This is an error');
    });

    it('should handle NOTIFICATION showType', () => {
      const error: any = new Error('Notification');
      error.name = 'BizError';
      error.info = {
        errorCode: 1004,
        errorMessage: 'This is a notification',
        showType: 3,
      };

      errorHandler(error, {});

      expect(feedback.notify).toHaveBeenCalledWith(
        1004,
        'This is a notification',
      );
    });

    it('should handle REDIRECT showType', () => {
      const error: any = new Error('Redirect');
      error.name = 'BizError';
      error.info = {
        errorCode: 401,
        errorMessage: 'Unauthorized',
        showType: 9,
      };

      errorHandler(error, {});

      // REDIRECT 分支不应触发任何消息/通知提示
      expect(feedback.warning).not.toHaveBeenCalled();
      expect(feedback.error).not.toHaveBeenCalled();
      expect(feedback.notify).not.toHaveBeenCalled();
    });

    it('should handle default case for unknown showType', () => {
      const error: any = new Error('Unknown type');
      error.name = 'BizError';
      error.info = {
        errorCode: 1005,
        errorMessage: 'Unknown error type',
        showType: 99,
      };

      errorHandler(error, {});

      expect(feedback.error).toHaveBeenCalledWith('Unknown error type');
    });

    it('should handle axios response error', () => {
      const error: any = new Error('Axios error');
      error.response = {
        status: 500,
        data: {},
      };

      errorHandler(error, {});

      expect(feedback.error).toHaveBeenCalledWith('Response status:500');
    });

    it('should handle offline error', () => {
      const error: any = new Error('Network error');
      error.request = {};

      const originalOnLine = navigator.onLine;
      Object.defineProperty(navigator, 'onLine', {
        writable: true,
        value: false,
      });

      try {
        errorHandler(error, {});

        expect(feedback.error).toHaveBeenCalledWith(
          '网络不可用，请检查连接后重试。',
        );
      } finally {
        Object.defineProperty(navigator, 'onLine', {
          writable: true,
          value: originalOnLine,
        });
      }
    });

    it('should handle request error with no response', () => {
      const error: any = new Error('Request error');
      error.request = {};

      errorHandler(error, {});

      expect(feedback.error).toHaveBeenCalledWith('服务未响应，请稍后重试。');
    });

    it('should handle generic error', () => {
      const error: any = new Error('Generic error');

      errorHandler(error, {});

      expect(feedback.error).toHaveBeenCalledWith('请求失败，请稍后重试。');
    });
  });
});
