import { useEffect, useMemo, useState } from "react";
import { Button, Empty, Input, Popover, Typography } from "antd";

type SearchResultItem = {
  key: string;
  title: string;
  subtitle?: string;
  onSelect: () => void;
};

const loadRecentSearches = (storageKey: string) => {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      return [] as string[];
    }
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [] as string[];
    }
    return parsed.filter((item): item is string => typeof item === "string").slice(0, 2);
  } catch {
    return [] as string[];
  }
};

const saveRecentSearches = (storageKey: string, keywords: string[]) => {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(keywords.slice(0, 2)));
  } catch {
    // Ignore storage failures.
  }
};

export const SearchInputDropdown = ({
  value,
  onChange,
  placeholder,
  storageKey,
  title,
  subtitle,
  emptyText,
  resultEmptyText,
  className,
  results,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  storageKey: string;
  title: string;
  subtitle: string;
  emptyText: string;
  resultEmptyText: string;
  className?: string;
  results: SearchResultItem[];
}) => {
  const [open, setOpen] = useState(false);
  const [recent, setRecent] = useState<string[]>([]);

  useEffect(() => {
    setRecent(loadRecentSearches(storageKey));
  }, [storageKey]);

  const persistKeyword = (keyword: string) => {
    const nextKeyword = keyword.trim();
    if (!nextKeyword) {
      return;
    }
    const nextRecent = [nextKeyword, ...recent.filter((item) => item !== nextKeyword)].slice(0, 2);
    setRecent(nextRecent);
    saveRecentSearches(storageKey, nextRecent);
  };

  const content = useMemo(() => {
    const hasKeyword = Boolean(value.trim());

    return (
      <div className="search-dropdown">
        <div className="search-dropdown-primary">
          <div className="search-dropdown-copy">
            <div className="search-dropdown-title">{title}</div>
            <div className="search-dropdown-subtitle">{subtitle}</div>
          </div>
        </div>
        {hasKeyword ? (
          <>
            <div className="search-dropdown-recent-header">
              <Typography.Text type="secondary">搜索结果</Typography.Text>
            </div>
            {results.length > 0 ? (
              <div className="search-dropdown-recent-list">
                {results.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className="search-dropdown-recent-item"
                    onClick={() => {
                      persistKeyword(value);
                      item.onSelect();
                      setOpen(false);
                    }}
                  >
                    <div className="search-dropdown-result-title">{item.title}</div>
                    {item.subtitle ? <div className="search-dropdown-result-subtitle">{item.subtitle}</div> : null}
                  </button>
                ))}
              </div>
            ) : (
              <div className="search-dropdown-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={resultEmptyText} />
              </div>
            )}
          </>
        ) : (
          <>
            <div className="search-dropdown-recent-header">
              <Typography.Text type="secondary">最近在搜</Typography.Text>
              {recent.length > 0 ? (
                <Button
                  type="link"
                  size="small"
                  onClick={() => {
                    setRecent([]);
                    saveRecentSearches(storageKey, []);
                  }}
                >
                  清空
                </Button>
              ) : null}
            </div>
            {recent.length > 0 ? (
              <div className="search-dropdown-recent-list">
                {recent.map((item) => (
                  <button
                    key={item}
                    type="button"
                    className="search-dropdown-recent-item"
                    onClick={() => {
                      onChange(item);
                    }}
                  >
                    <div className="search-dropdown-result-title">{item}</div>
                  </button>
                ))}
              </div>
            ) : (
              <div className="search-dropdown-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />
              </div>
            )}
          </>
        )}
      </div>
    );
  }, [emptyText, onChange, recent, resultEmptyText, results, storageKey, subtitle, title, value]);

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      trigger="click"
      placement="bottomLeft"
      overlayClassName="search-dropdown-popover"
      content={content}
    >
      <Input
        value={value}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onChange={(event) => onChange(event.target.value)}
        onPressEnter={() => persistKeyword(value)}
        placeholder={placeholder}
        allowClear
        className={className}
      />
    </Popover>
  );
};
