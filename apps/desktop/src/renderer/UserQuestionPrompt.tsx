import { useEffect, useRef, useState } from "react";
import { AwakenButton as Button, AwakenIcon } from "@awaken/design-system";
import type { UserQuestionItem } from "./runtime/reducer";
type UserQuestionPromptProps = {
  question: UserQuestionItem;
  disabled?: boolean;
  onCancel: () => void;
  onSubmit: (answer: string) => void;
};

export type UserQuestionSelection = {
  selected: string[];
  otherText: string;
};

const USER_QUESTION_OTHER_OPTION = "__other__";

const emptyUserQuestionSelection = (): UserQuestionSelection => ({
  selected: [],
  otherText: "",
});

export function selectedUserQuestionLabels(
  selection: UserQuestionSelection,
): string[] {
  const labels = selection.selected.filter(
    (label) => label !== USER_QUESTION_OTHER_OPTION,
  );
  if (
    selection.selected.includes(USER_QUESTION_OTHER_OPTION) &&
    selection.otherText.trim()
  ) {
    labels.push(selection.otherText.trim());
  }
  return labels;
}

export function isUserQuestionSelectionAnswered(
  selection: UserQuestionSelection,
): boolean {
  return selectedUserQuestionLabels(selection).length > 0;
}

export function nextUserQuestionPromptIndex(
  currentIndex: number,
  questionCount: number,
): number {
  if (questionCount <= 0) {
    return 0;
  }
  return Math.min(currentIndex + 1, questionCount - 1);
}

function clampUserQuestionPromptIndex(
  index: number,
  questionCount: number,
): number {
  if (questionCount <= 0) {
    return 0;
  }
  return Math.min(Math.max(index, 0), questionCount - 1);
}

export function UserQuestionPrompt({
  question,
  disabled,
  onCancel,
  onSubmit,
}: UserQuestionPromptProps) {
  const [answers, setAnswers] = useState<Record<number, UserQuestionSelection>>(
    {},
  );
  const [activeQuestionIndex, setActiveQuestionIndex] = useState(0);
  const otherInputRefs = useRef<Record<number, HTMLInputElement | null>>({});
  const questionBodyRef = useRef<HTMLDivElement | null>(null);
  const questions = question.questions;

  useEffect(() => {
    setAnswers({});
    setActiveQuestionIndex(0);
  }, [question.id, question.withdrawn]);

  useEffect(() => {
    setActiveQuestionIndex((current) =>
      clampUserQuestionPromptIndex(current, questions.length),
    );
  }, [questions.length]);

  useEffect(() => {
    if (questionBodyRef.current) {
      questionBodyRef.current.scrollTop = 0;
    }
  }, [question.id, activeQuestionIndex]);

  if (questions.length === 0) {
    return null;
  }

  if (question.withdrawn) {
    return (
      <div className="user-question-card" aria-label="已撤回的问题">
        <div className="user-question-card__topbar">
          <div className="user-question-card__header" role="status">正在停止，问题已撤回</div>
        </div>
        <div className="user-question-card__body" role="region" aria-label="原问题" tabIndex={0}>
          {questions.map((item, index) => (
            <div className="user-question-card__question" key={index}>
              <div className="user-question-card__title">{item.question}</div>
              {item.options?.map((option) => (
                <div key={option.label} className="user-question-card__option">
                  <span className="user-question-card__option-label">{option.label}</span>
                  <span className="user-question-card__option-description">{option.description}</span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    );
  }

  const questionCount = questions.length;
  const currentPageIndex = clampUserQuestionPromptIndex(
    activeQuestionIndex,
    questionCount,
  );
  const activeQuestion = questions[currentPageIndex];
  const hasPreviousQuestion = currentPageIndex > 0;
  const hasNextQuestion = currentPageIndex < questionCount - 1;
  const hasMultipleQuestions = questionCount > 1;

  const currentAnswer = (index: number): UserQuestionSelection =>
    answers[index] ?? emptyUserQuestionSelection();

  const isSelected = (index: number, label: string): boolean =>
    currentAnswer(index).selected.includes(label);

  const goToQuestion = (index: number) => {
    setActiveQuestionIndex(clampUserQuestionPromptIndex(index, questionCount));
  };

  const goToPreviousQuestion = () => {
    goToQuestion(currentPageIndex - 1);
  };

  const goToNextQuestion = () => {
    goToQuestion(currentPageIndex + 1);
  };

  const toggleOption = (index: number, multiSelect: boolean, label: string) => {
    setAnswers((current) => {
      const entry = current[index] ?? emptyUserQuestionSelection();
      let selected: string[];
      if (multiSelect) {
        selected = entry.selected.includes(label)
          ? entry.selected.filter((item) => item !== label)
          : [...entry.selected, label];
      } else {
        selected = [label];
      }
      return {
        ...current,
        [index]: { ...entry, selected },
      };
    });
    if (label === USER_QUESTION_OTHER_OPTION) {
      window.setTimeout(() => otherInputRefs.current[index]?.focus(), 0);
    }
    if (!multiSelect && label !== USER_QUESTION_OTHER_OPTION) {
      window.setTimeout(() => {
        setActiveQuestionIndex((current) =>
          current === index
            ? nextUserQuestionPromptIndex(index, questionCount)
            : current,
        );
      }, 120);
    }
  };

  const setOtherText = (index: number, value: string) => {
    setAnswers((current) => {
      const entry = current[index] ?? emptyUserQuestionSelection();
      return {
        ...current,
        [index]: { ...entry, otherText: value },
      };
    });
  };

  const selectedLabelsForQuestion = (index: number): string[] => {
    return selectedUserQuestionLabels(currentAnswer(index));
  };

  const canSubmit = questions.every(
    (_item, index) => selectedLabelsForQuestion(index).length > 0,
  );
  const currentQuestionAnswered = isUserQuestionSelectionAnswered(
    currentAnswer(currentPageIndex),
  );
  const answeredCount = questions.filter((_item, index) =>
    isUserQuestionSelectionAnswered(currentAnswer(index)),
  ).length;

  const handleSubmit = () => {
    if (disabled || !canSubmit) {
      return;
    }
    onSubmit(
      questions
        .map((item, index) => {
          const labels = selectedLabelsForQuestion(index);
          return `${item.question}: ${labels.join(", ")}`;
        })
        .join("\n"),
    );
  };

  const handlePrimaryAction = () => {
    if (disabled) {
      return;
    }
    if (hasNextQuestion) {
      if (currentQuestionAnswered) {
        goToNextQuestion();
      }
      return;
    }
    handleSubmit();
  };

  const isMulti = activeQuestion.multiSelect ?? false;
  const options = Array.isArray(activeQuestion.options)
    ? activeQuestion.options
    : [];
  const hasOther = isSelected(currentPageIndex, USER_QUESTION_OTHER_OPTION);
  const primaryActionDisabled =
    disabled || (hasNextQuestion ? !currentQuestionAnswered : !canSubmit);
  const primaryActionLabel = hasNextQuestion ? "下一题" : "提交回答";

  return (
    <div
      className="user-question-card"
      onKeyDown={(event) => {
        if (event.key !== "Escape" || event.defaultPrevented || event.nativeEvent.isComposing || disabled || question.withdrawn) return;
        event.preventDefault();
        event.stopPropagation();
        onCancel();
      }}
      aria-label={`需要回答的问题 ${currentPageIndex + 1} of ${questionCount}`}
    >
      <div className="user-question-card__topbar">
        <div className="user-question-card__heading">
          <div className="user-question-card__header">{question.withdrawn ? "正在停止，问题已撤回" : "需要回答"}</div>
        </div>
        <div className="user-question-card__topbar-actions">
          {hasMultipleQuestions ? (
            <div className="user-question-card__pager" aria-label="问题切换">
              <button
                type="button"
                className="user-question-card__pager-button"
                aria-label="上一题"
                title="上一题"
                onClick={goToPreviousQuestion}
                disabled={disabled || !hasPreviousQuestion}
              >
                <AwakenIcon concept="collapse-up" size={16} />
              </button>
              <span className="user-question-card__counter" aria-live="polite">
                {currentPageIndex + 1} of {questionCount}
              </span>
              <button
                type="button"
                className="user-question-card__pager-button"
                aria-label="下一题"
                title="下一题"
                onClick={goToNextQuestion}
                disabled={disabled || !hasNextQuestion}
              >
                <AwakenIcon concept="dropdown" size={16} />
              </button>
            </div>
          ) : null}
          <button
            type="button"
            className="user-question-card__close"
            aria-label="取消提问"
            title="取消提问"
            onClick={onCancel}
            disabled={disabled}
          >
            <AwakenIcon concept="close" size={20} />
          </button>
        </div>
      </div>
      <div
        className="user-question-card__body"
        ref={questionBodyRef}
        role="region"
        aria-label="问题与选项"
        tabIndex={0}
      >
        <div
          className="user-question-card__question"
          key={`${currentPageIndex}:${activeQuestion.question}`}
        >
          {activeQuestion.header ? (
            <div className="user-question-card__header">
              {activeQuestion.header}
            </div>
          ) : null}
          <div className="user-question-card__title">
            {activeQuestion.question}
          </div>
          <div className="user-question-card__options">
            {options.map((option) => (
              <button
                key={option.label}
                type="button"
                className={`user-question-card__option${
                  isSelected(currentPageIndex, option.label)
                    ? " user-question-card__option--selected"
                    : ""
                }`}
                onClick={() =>
                  toggleOption(currentPageIndex, isMulti, option.label)
                }
                disabled={disabled}
              >
                <span className="user-question-card__option-label">
                  {option.label}
                </span>
                {option.description ? (
                  <span className="user-question-card__option-description">
                    {option.description}
                  </span>
                ) : null}
              </button>
            ))}
            <div
              className={`user-question-card__option user-question-card__option--other${
                hasOther ? " user-question-card__option--selected" : ""
              }${disabled ? " user-question-card__option--disabled" : ""}`}
              onClick={() => {
                if (!disabled) {
                  toggleOption(
                    currentPageIndex,
                    isMulti,
                    USER_QUESTION_OTHER_OPTION,
                  );
                }
              }}
              onKeyDown={(event) => {
                if (disabled || (event.key !== "Enter" && event.key !== " ")) {
                  return;
                }
                event.preventDefault();
                toggleOption(
                  currentPageIndex,
                  isMulti,
                  USER_QUESTION_OTHER_OPTION,
                );
              }}
              role="button"
              aria-pressed={hasOther}
              tabIndex={disabled ? -1 : 0}
            >
              <span className="user-question-card__option-label">
                其他（输入你的答案）
              </span>
              {hasOther ? (
                <input
                  ref={(node) => {
                    otherInputRefs.current[currentPageIndex] = node;
                  }}
                  type="text"
                  className="user-question-card__other-input"
                  placeholder="请输入你的回答..."
                  value={currentAnswer(currentPageIndex).otherText}
                  onChange={(event) =>
                    setOtherText(currentPageIndex, event.target.value)
                  }
                  disabled={disabled}
                  onClick={(event) => event.stopPropagation()}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") return;
                    event.stopPropagation();
                    if (event.nativeEvent.isComposing || event.key !== "Enter") {
                      return;
                    }
                    event.preventDefault();
                    handlePrimaryAction();
                  }}
                />
              ) : null}
            </div>
          </div>
        </div>
      </div>
      <div className="user-question-card__actions">
        {hasMultipleQuestions ? (
          <span className="user-question-card__answer-status">
            已回答 {answeredCount} / {questionCount}
          </span>
        ) : null}
        <Button
          variant="default"
          size="sm"
          disabled={disabled}
          onClick={onCancel}
        >
          取消
        </Button>
        <Button
          variant="primary"
          size="sm"
          loading={disabled}
          disabled={primaryActionDisabled}
          onClick={() => {
            handlePrimaryAction();
          }}
        >
          {primaryActionLabel}
        </Button>
      </div>
    </div>
  );
}
